package controllers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	jangofettv1alpha1 "gitlab.com/beecash/infra/jango-fett/api/v1alpha1"
)

var _ = Describe("PostgreSQL controller", func() {

	const (
		PostgreSQLObjName      = "test-environment"
		PostgreSQLObjNamespace = "default"

		host          = "localhost"
		port          = "5432"
		adminUsername = "postgres"
		adminPassword = "postgres"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	var (
		jfDatabaseName = generateDatabaseName(PostgreSQLObjNamespace, PostgreSQLObjName)
		jfDatabaseUser = generateDatabaseUser(PostgreSQLObjNamespace, PostgreSQLObjName)
		postgresURL    = fmt.Sprintf("postgres://%s:%s@%s:%s?sslmode=disable", adminUsername, adminPassword, host, port)
	)

	Context("PostgreSQL object lifecycle", func() {
		ctx := context.Background()
		postgresObj := &jangofettv1alpha1.PostgreSQL{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "jango-fett.bukukas.io/v1alpha1",
				Kind:       "PostgreSQL",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      PostgreSQLObjName,
				Namespace: PostgreSQLObjNamespace,
			},
			Spec: jangofettv1alpha1.PostgreSQLSpec{},
		}

		It("Should accept the PostgreSQL CRD", func() {
			By("By creating a new PostgreSQL Object")
			Expect(k8sClient.Create(ctx, postgresObj)).Should(Succeed())
		})

		It("Should create all the resources", func() {

			By("Creating Secret object")
			secret := &corev1.Secret{}
			secretLookupKey := types.NamespacedName{Name: "jf-postgresql-" + PostgreSQLObjName, Namespace: PostgreSQLObjNamespace}

			// We'll need to retry getting this newly created Namespacee, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, secretLookupKey, secret)
				if err != nil {
					fmt.Println(err)
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			Expect(string(secret.Data["DATABASE"])).Should(Equal(jfDatabaseName))
			Expect(string(secret.Data["USERNAME"])).Should(Equal(jfDatabaseUser))

			By("Creating database with user")
			db, err := sql.Open("postgres", postgresURL)
			if err != nil {
				panic(err)
			}
			defer db.Close()
			selectDBQuery := `SELECT datdba::regrole FROM pg_database WHERE datname = '%s' LIMIT 1`

			var actualOwner string
			db.QueryRow(fmt.Sprintf(selectDBQuery, jfDatabaseName)).Scan(&actualOwner)
			Expect(actualOwner).Should(Equal(jfDatabaseUser))

			By("Checking status of PostgreSQL object")
			Eventually(func() bool {
				updatedPostgresObj := &jangofettv1alpha1.PostgreSQL{}
				updatedPostgresObjLookupKey := types.NamespacedName{
					Name:      PostgreSQLObjName,
					Namespace: PostgreSQLObjNamespace,
				}
				err := k8sClient.Get(ctx, updatedPostgresObjLookupKey, updatedPostgresObj)
				if err != nil {
					fmt.Println(err)
					return false
				}
				return updatedPostgresObj.Status.Ready
			}, timeout, interval).Should(BeTrue())

		})

		It("Should clean up objects when PostgreSQL object is deleted", func() {

			By("Expecting PostgreSQL Object to delete successfully")
			Eventually(func() error {
				err := k8sClient.Delete(ctx, postgresObj)
				return err
			}, timeout, interval).Should(Succeed())

			By("Expecting Secret to be deleted")
			Eventually(func() error {
				secret := &corev1.Secret{}
				secretLookupKey := types.NamespacedName{Name: "jf-postgresql", Namespace: PostgreSQLObjNamespace}
				return k8sClient.Get(ctx, secretLookupKey, secret)
			}, timeout, interval).ShouldNot(Succeed())

			By("Expecting Database and user to be deleted")
			Eventually(func() error {
				db, err := sql.Open("postgres", postgresURL)
				if err != nil {
					return err
				}

				var dbCount int
				fmt.Println("doing db select")
				err = db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM pg_database WHERE datname = '%s'`, jfDatabaseName)).Scan(&dbCount)
				if err != nil {
					return err
				}
				if dbCount != 0 {
					return errors.New("database not deleted")
				}

				var userCount int
				fmt.Println("doing user select")
				err = db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM pg_roles WHERE rolname = '%s'`, jfDatabaseUser)).Scan(&userCount)
				if err != nil {
					return err
				}
				if userCount != 0 {
					return errors.New("user not deleted")
				}

				return nil
			}, timeout, interval).Should(Succeed())
		})
	})
})
