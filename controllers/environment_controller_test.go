package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	jangofettv1alpha1 "gitlab.com/beecash/infra/jango-fett/api/v1alpha1"
)

var _ = Describe("Environment controller", func() {

	const (
		EnvironmentName      = "test-environment"
		EnvironmentNamespace = "default"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("Environment object lifecycle", func() {
		ctx := context.Background()
		envObj := &jangofettv1alpha1.Environment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "jango-fett.bukukas.io/v1alpha1",
				Kind:       "Environment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      EnvironmentName,
				Namespace: EnvironmentNamespace,
			},
			Spec: jangofettv1alpha1.EnvironmentSpec{
				Config: map[string]string{
					"foo": "bar",
					"bar": "baz",
				},
				Users: []string{
					"yash@beecash.io",
					"madhukar@beecash.io",
				},
			},
		}
		envLookupNamespace := generatedNamespace(EnvironmentName)
		nsLookupKey := types.NamespacedName{Name: envLookupNamespace}

		It("Should have all required things in the namespace", func() {
			By("By creating a new Environment")
			Expect(k8sClient.Create(ctx, envObj)).Should(Succeed())
		})

		It("Should create supporting objects in the Namespace", func() {

			By("Creating Namespace")
			createdNamespace := &corev1.Namespace{}

			// We'll need to retry getting this newly created Namespacee, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nsLookupKey, createdNamespace)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			expectedNamespace := envNamespaceObject(*envObj)
			Expect(createdNamespace.Name).Should(Equal(expectedNamespace.Name))
			Expect(createdNamespace.Labels["created-by"]).Should(Equal("yash.beecash.io"))

			By("Creating ConfigMap")
			cmLookupKey := types.NamespacedName{Name: "jango-fett", Namespace: envLookupNamespace}
			createdConfigMap := &corev1.ConfigMap{}

			// We'll need to retry getting this newly created ConfigMap, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, cmLookupKey, createdConfigMap)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// ConfigMap that is created should have the same Data as defined in EnvironmentSpec's Config field
			Expect(createdConfigMap.Data).Should(Equal(envObj.Spec.Config))

			By("Creating RoleBinding")
			rbLookupKey := types.NamespacedName{Name: "jango-fett-users", Namespace: envLookupNamespace}
			createdRoleBinding := &rbacv1.RoleBinding{}

			// We'll need to retry getting this newly created RoleBinding, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, rbLookupKey, createdRoleBinding)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			expectedRoleBinding := envRoleBindingObject(*envObj, envLookupNamespace)

			// RoleBinding object's users should be the same as defined in EnvironmentSpec's users
			var subjectUsers []string
			for _, subject := range createdRoleBinding.Subjects {
				subjectUsers = append(subjectUsers, subject.Name)
			}
			Expect(subjectUsers).Should(Equal(envObj.Spec.Users))
			Expect(createdRoleBinding.RoleRef).Should(Equal(expectedRoleBinding.RoleRef))

			By("Checking status of Environment object")
			Eventually(func() bool {
				updatedEnvObj := &jangofettv1alpha1.Environment{}
				updatedEnvObjLookupKey := types.NamespacedName{
					Name:      EnvironmentName,
					Namespace: EnvironmentNamespace,
				}
				err := k8sClient.Get(ctx, updatedEnvObjLookupKey, updatedEnvObj)
				if err != nil {
					fmt.Println(err)
					return false
				}
				return updatedEnvObj.Status.Ready
			}, timeout, interval).Should(BeTrue())

		})

		It("Should clean up objects when Environment is deleted", func() {

			By("Expecting Environment to delete successfully")
			Eventually(func() error {
				err := k8sClient.Delete(ctx, envObj)
				return err
			}, timeout, interval).Should(Succeed())

			By("Expecting Environment to be deleted")
			envLookupKey := types.NamespacedName{
				Name:      EnvironmentName,
				Namespace: EnvironmentNamespace,
			}
			Eventually(func() error {
				return k8sClient.Get(ctx, envLookupKey, envObj)
			}, timeout, interval).ShouldNot(Succeed())

			By("Expecting Namespace to be terminated")

			// Since there are no controllers monitoring built-in resources, objects do not get deleted.
			// Hence we are checking whether the status changes to Terminating
			Eventually(func() corev1.NamespacePhase {
				ns := &corev1.Namespace{}
				_ = k8sClient.Get(ctx, nsLookupKey, ns)
				return ns.Status.Phase
			}, timeout, interval).Should(Equal(corev1.NamespaceTerminating))

		})
	})
})
