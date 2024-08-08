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
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"

	jangofettv1alpha1 "gitlab.com/beecash/infra/jango-fett/api/v1alpha1"
	"gitlab.com/beecash/infra/jango-fett/kutils"
)

// EnvironmentReconciler reconciles a Environment object
type EnvironmentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// generatedNamespace returns the name of the namespace that an Environment should create
func generatedNamespace(envName string) string {
	return "jf-" + envName
}

// envNamespaceObject returns a Namespace for the Environment CRD
func envNamespaceObject(envObj jangofettv1alpha1.Environment) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: generatedNamespace(envObj.Name),
			Labels: map[string]string{
				"managed-by": "jango-fett",
				"created-by": strings.ReplaceAll(envObj.Spec.Users[0], "@", "."),
			},
		},
	}
}

// envConfigMapObject returns a ConfigMap which takes key/values from EnvironmentSpec.Config
func envConfigMapObject(envObj jangofettv1alpha1.Environment, namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "jango-fett",
			Namespace: namespace,
		},
		Data: envObj.Spec.Config,
	}

}

// envRoleBinding object returns a RoleBinding which gives edit access all users in EnvironmentSpec.Users
func envRoleBindingObject(envObj jangofettv1alpha1.Environment, namespace string) *rbacv1.RoleBinding {
	var subjects []rbacv1.Subject
	for _, user := range envObj.Spec.Users {
		subjects = append(subjects, rbacv1.Subject{
			Kind:     "User",
			Name:     user,
			APIGroup: "rbac.authorization.k8s.io",
		})
	}
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "jango-fett-users",
			Namespace: namespace,
		},
		Subjects: subjects,
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
}

//+kubebuilder:rbac:groups=jango-fett.bukukas.io,resources=environments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=jango-fett.bukukas.io,resources=environments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=jango-fett.bukukas.io,resources=environments/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=roles;rolebindings;clusterroles,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *EnvironmentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrlLog.FromContext(ctx)

	envObj := &jangofettv1alpha1.Environment{}

	// Check if the environment CRD exists
	err := r.Get(ctx, req.NamespacedName, envObj)
	if err != nil {
		if errors.IsNotFound(err) {
			// Since environment object does not exist, it is implied that its deleted
			// All the objects are namespace scoped so deleting the namespace
			// cleans up the environment
			log.V(0).Info("Environment CRD does not exist",
				"name", req.NamespacedName.Name,
				"namespace", req.NamespacedName.Namespace)

			// Create namespace object to destroye it
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: generatedNamespace(req.NamespacedName.Name),
				},
			}
			_, err = kutils.KubeDestroy(r, ctx, ns)
			if err != nil {
				log.Error(err, "Unable to delete Namespace", "namespace", ns.Name)
				return ctrl.Result{}, err
			}
			log.V(0).Info("Namespace deleted", "namespace", ns.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Apply Namespace object
	ns := envNamespaceObject(*envObj)
	_, err = kutils.KubeApply(r, ctx, ns)
	if err != nil {
		log.Error(err, "Unable to create Namespace", "namespace", ns.Name)
		return ctrl.Result{}, err
	}
	log.V(0).Info("Namespace applied", "namespace", ns.Name)

	// Apply ConfigMap object
	cm := envConfigMapObject(*envObj, ns.Name)
	_, err = kutils.KubeApply(r, ctx, cm)
	if err != nil {
		log.Error(err, "Unable to apply ConfigMap",
			"name", cm.Name,
			"namespace", cm.Namespace)
		return ctrl.Result{}, err
	}
	log.V(0).Info("ConfigMap applied",
		"name", cm.Name,
		"namespace", cm.Namespace)

	// Apply RoleBinding object
	rb := envRoleBindingObject(*envObj, ns.Name)
	_, err = kutils.KubeApply(r, ctx, rb)
	if err != nil {
		log.Error(err, "Unable to apply RoleBinding",
			"name", rb.Name,
			"namespace", rb.Namespace)
		return ctrl.Result{}, err
	}
	log.V(0).Info("RoleBinding applied",
		"name", rb.Name,
		"namespace", rb.Namespace)

	// Update status of PostgreSQL object
	envObj.Status.Ready = true
	if err := r.Status().Update(ctx, envObj); err != nil {
		log.Error(err, "unable to update Environment status",
			"name", envObj.Name,
			"namespace", envObj.Namespace)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EnvironmentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&jangofettv1alpha1.Environment{}).
		Complete(r)
}
