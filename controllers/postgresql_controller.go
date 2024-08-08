/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"regexp"
	"strings"

	_ "github.com/lib/pq"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"

	jangofettv1alpha1 "gitlab.com/beecash/infra/jango-fett/api/v1alpha1"
	"gitlab.com/beecash/infra/jango-fett/kutils"
)

// PostgreSQLReconciler reconciles a PostgreSQL object
type PostgreSQLReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func postgresSecretObject(metadataName, namespace, host, port, jfDatabaseName, jfDatabaseUser, password string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "jf-postgresql-" + metadataName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"HOST":     []byte(host),
			"PORT":     []byte(port),
			"USERNAME": []byte(jfDatabaseUser),
			"DATABASE": []byte(jfDatabaseName),
			"PASSWORD": []byte(password),
		},
	}
}

func CreateDatabaseWithUser(ctx context.Context, postgresURL, dbName, user, password string) error {

	log := ctrlLog.FromContext(ctx)
	db, err := sql.Open("postgres", postgresURL+"?sslmode=disable")
	if err != nil {
		return err
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		return err
	}

	createUserQuery := `CREATE USER %s WITH PASSWORD '%s';`
	query := fmt.Sprintf(createUserQuery, user, password)
	_, err = db.Exec(query)
	if err != nil {
		// If user already exists, lets ignore that case, else return error
		if !strings.HasSuffix(err.Error(), "already exists") {
			return err
		}
	}
	log.V(0).Info("Created PostgreSQL user",
		"username", user)

	// Check if DB exists
	var count int
	selectDBQuery := `SELECT count(*) FROM pg_database WHERE datname = $1`
	err = db.QueryRow(selectDBQuery, dbName).Scan(&count)
	if err != nil {
		return err
	}

	if count != 0 {
		log.V(0).Info("Database already exists, aborting creation",
			"database", dbName)
		return nil
	}

	// Grant role
	grantRoleQuery := `GRANT %s TO postgres;`
	_, err = db.Exec(fmt.Sprintf(grantRoleQuery, user))
	if err != nil {
		return err
	}
	log.V(0).Info("Granted role to postgres",
		"role", user)

	createDBQuery := `CREATE DATABASE %s OWNER %s`
	_, err = db.Exec(fmt.Sprintf(createDBQuery, dbName, user))
	if err != nil {
		return err
	}
	log.V(0).Info("Created PostgreSQL database with owner",
		"database", dbName,
		"username", user)

	// Connect to jf database for adding extensions
	jfDb, err := sql.Open("postgres", postgresURL+fmt.Sprintf("/%s?sslmode=disable", dbName))
	if err != nil {
		return err
	}
	defer jfDb.Close()

	createUUIDExtensionQuery := `CREATE EXTENSION IF NOT EXISTS "uuid-ossp";`
	_, err = jfDb.Exec(createUUIDExtensionQuery)
	if err != nil {
		return err
	}

	log.V(0).Info("Created uuid-ossp extension",
		"database", dbName,
		"username", user)

	return nil
}

func DeleteDatabaseWithUser(ctx context.Context, postgresURL, dbName, user string) error {

	log := ctrlLog.FromContext(ctx)
	db, err := sql.Open("postgres", postgresURL)
	if err != nil {
		return err
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		return err
	}

	log.V(0).Info("Deleting PostgreSQL database",
		"database", dbName)
	deleteDBQuery := `DROP DATABASE IF EXISTS %s`
	_, err = db.Exec(fmt.Sprintf(deleteDBQuery, dbName))
	if err != nil {
		return err
	}

	log.V(0).Info("Deleting PostgreSQL user",
		"username", user)
	dropUserQuery := `
        REASSIGN OWNED BY %[1]s TO postgres;
        DROP OWNED BY %[1]s;
        DROP USER %[1]s;
    `
	_, err = db.Exec(fmt.Sprintf(dropUserQuery, user))
	if err != nil {
		return err
	}

	return nil
}

func requiredEnvVars(envVars []string) (map[string]string, error) {
	missing := []string{}
	keyVals := map[string]string{}
	for _, key := range envVars {
		if _, exists := os.LookupEnv(key); !exists {
			missing = append(missing, key)
		} else {
			keyVals[key] = os.Getenv(key)
		}
	}
	if len(missing) > 0 {
		return nil, errors.New(
			fmt.Sprintf("Unset required ENV vars: %s", strings.Join(missing, ", ")))
	}
	return keyVals, nil
}

func randomString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

	s := make([]rune, n)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

// Database name convention: jf_<namespace>_<metadata.name>
func generateDatabaseName(namespace, objName string) string {
	uid := fmt.Sprintf("jf_%s_%s", strings.Replace(namespace, "-", "_", -1), strings.Replace(objName, "-", "_", -1))
	jfDatabaseName := uid
	re := regexp.MustCompile("^[a-zA-Z_]+[0-9a-zA-Z_]*$")
	if !re.MatchString(jfDatabaseName) {
		panic(errors.New("database name regex check failed"))
	}
	return jfDatabaseName
}

// Database username convention: jf_<namespace>_<metadata.name>
func generateDatabaseUser(namespace, objName string) string {
	uid := fmt.Sprintf("jf_%s_%s", strings.Replace(namespace, "-", "_", -1), strings.Replace(objName, "-", "_", -1))
	jfDatabaseUser := uid
	re := regexp.MustCompile("^[a-zA-Z_]+[0-9a-zA-Z_]*$")

	if !re.MatchString(jfDatabaseUser) {
		panic(errors.New("database user regex check failed"))
	}
	return jfDatabaseUser
}

//+kubebuilder:rbac:groups=jango-fett.bukukas.io,resources=postgresqls,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=jango-fett.bukukas.io,resources=postgresqls/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=jango-fett.bukukas.io,resources=postgresqls/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *PostgreSQLReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrlLog.FromContext(ctx)

	envVars, err := requiredEnvVars([]string{
		"POSTGRESQL_HOST", "POSTGRESQL_PORT", "POSTGRESQL_USERNAME", "POSTGRESQL_PASSWORD"})

	if err != nil {
		log.Error(err, "Required env vars have not been set")
		panic(err)
	}

	host := envVars["POSTGRESQL_HOST"]
	port := envVars["POSTGRESQL_PORT"]
	adminUsername := envVars["POSTGRESQL_USERNAME"]
	adminPassword := envVars["POSTGRESQL_PASSWORD"]

	postgresqlURL := fmt.Sprintf("postgres://%s:%s@%s:%s", adminUsername, url.QueryEscape(adminPassword), host, port)

	ns := req.NamespacedName.Namespace
	objName := req.NamespacedName.Name

	// Database name and username convention: jf_<namespace>_<metadata.name>
	uid := fmt.Sprintf("jf_%s_%s", strings.Replace(ns, "-", "_", -1), strings.Replace(objName, "-", "_", -1))
	jfDatabaseName := uid
	jfDatabaseUser := uid
	re := regexp.MustCompile("^[a-zA-Z_]+[0-9a-zA-Z_]*$")
	if !re.MatchString(jfDatabaseName) {
		return ctrl.Result{}, errors.New("database name regex check failed")
	}

	if !re.MatchString(jfDatabaseUser) {
		return ctrl.Result{}, errors.New("database user regex check failed")
	}

	// Generate password
	password := randomString(10)

	dbObj := &jangofettv1alpha1.PostgreSQL{}

	secret := postgresSecretObject(objName, ns, host, port, jfDatabaseName, jfDatabaseUser, password)
	err = r.Get(ctx, req.NamespacedName, dbObj)
	if err != nil {
		if k8sErrors.IsNotFound(err) {
			// Since database object does not exist, it is implied that its deleted
			// Now we need to delete the database and the user
			// also remove secret
			err = DeleteDatabaseWithUser(ctx, postgresqlURL, jfDatabaseName, jfDatabaseUser)
			if err != nil {
				log.Error(err, "Error when deleting database and user",
					"database_name", jfDatabaseName,
					"database_user", jfDatabaseUser)
				return ctrl.Result{}, err
			}
			_, err = kutils.KubeDestroy(r, ctx, secret)
			if err != nil {
				log.Error(err, "Unable to delete Secret", "secret", secret.Name)
				return ctrl.Result{}, err
			}

			log.V(0).Info("Secret deleted",
				"name", secret.Name,
				"namespace", secret.Namespace)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	err = CreateDatabaseWithUser(ctx, postgresqlURL, jfDatabaseName, jfDatabaseUser, password)
	if err != nil {
		log.Error(err, "Error when creating database and user",
			"database_name", jfDatabaseName,
			"database_user", jfDatabaseUser)
		return ctrl.Result{}, err
	}

	_, err = kutils.KubeApply(r, ctx, secret)
	if err != nil {
		log.Error(err, "Unable to apply Secret",
			"name", secret.Name,
			"namespace", secret.Namespace)
		return ctrl.Result{}, err
	}
	log.V(0).Info("Secret applied",
		"name", secret.Name,
		"namespace", secret.Namespace)

	// Update status of PostgreSQL object
	dbObj.Status.Ready = true
	if err := r.Status().Update(ctx, dbObj); err != nil {
		log.Error(err, "unable to update PostgreSQL status",
			"name", dbObj.Name,
			"namespace", dbObj.Namespace)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PostgreSQLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&jangofettv1alpha1.PostgreSQL{}).
		Complete(r)
}
