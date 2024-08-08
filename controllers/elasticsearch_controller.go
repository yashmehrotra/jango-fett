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
	"encoding/json"
	"fmt"

	"github.com/olivere/elastic/v7"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	jangofettv1alpha1 "gitlab.com/beecash/infra/jango-fett/api/v1alpha1"
	"gitlab.com/beecash/infra/jango-fett/kutils"
)

// ElasticsearchReconciler reconciles a Elasticsearch object
type ElasticsearchReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

type index struct {
	client *elastic.Client
	name   string
}

// Create index in elasticsearch if it does not already exist
func (i *index) create(ctx context.Context) error {
	exists, err := i.client.IndexExists(i.name).Do(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	if _, err := i.client.CreateIndex(i.name).Do(ctx); err != nil {
		return err
	}
	return nil
}

// Delete index in elasticsearch
func (i *index) delete(ctx context.Context) error {
	if _, err := i.client.DeleteIndex(i.name).Do(ctx); err != nil {
		return err
	}
	return nil
}

func newIndex(name string, client *elastic.Client) *index {
	return &index{
		name:   name,
		client: client,
	}
}

//+kubebuilder:rbac:groups=jango-fett.bukukas.io,resources=elasticsearches,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=jango-fett.bukukas.io,resources=elasticsearches/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=jango-fett.bukukas.io,resources=elasticsearches/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *ElasticsearchReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	envVars, err := requiredEnvVars([]string{"ELASTICSEARCH_URL"})

	if err != nil {
		log.Error(err, "Required env vars have not been set")
		panic(err)
	}

	esUrl := envVars["ELASTICSEARCH_URL"]
	es, err := elastic.NewClient(elastic.SetURL(esUrl), elastic.SetSniff(false))
	if err != nil {
		log.Error(err, "Error initializing Elasticsearch client")
		return ctrl.Result{}, err
	}

	esCtx := context.TODO()

	if pingRes, _, err := es.Ping(esUrl).Do(esCtx); err != nil {
		log.Error(err, "Pinging Elasticsearch failed")
	} else {
		if _, err := json.Marshal(pingRes); err != nil {
			log.Error(err, "Elasticsearch ping response could not be serialized",
				"response", pingRes)
			return ctrl.Result{}, err
		} else {
			log.V(0).Info("Connected to Elasticsearch")
		}
	}

	ns := req.NamespacedName.Namespace
	objName := req.NamespacedName.Name

	esObj := &jangofettv1alpha1.Elasticsearch{}
	indexName := fmt.Sprintf("jf-%s-%s", ns, objName)
	esIndex := newIndex(indexName, es)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "jf-elasticsearch-" + objName,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"ELASTICSEARCH_URL": []byte(esUrl),
			"ES_INDEX_NAME":     []byte(indexName),
		},
	}
	// If reconcile is called with a request for an object that doesn't exist,
	// it is deleted, so we'll delete the corresponding ES index
	if k8sErrors.IsNotFound(r.Get(ctx, req.NamespacedName, esObj)) {
		if err := esIndex.delete(esCtx); err != nil {
			log.Error(err, "Error when deleting index",
				"index", indexName)
			return ctrl.Result{}, err
		}
		if _, err := kutils.KubeDestroy(r, ctx, secret); err != nil {
			log.Error(err, "Unable to delete Secret",
				"name", secret.Name,
				"namespace", secret.Namespace)
			return ctrl.Result{}, err
		}
		log.V(0).Info("Secret deleted",
			"name", secret.Name,
			"namespace", secret.Namespace)
		return ctrl.Result{}, nil
	}

	// We're not handling updates since now the spec doesn't have any data right now.
	// So no need to handle updates, only create.
	if err := esIndex.create(esCtx); err != nil {
		log.Error(err, "Failed to create index",
			"index", indexName)
		return ctrl.Result{}, err
	}
	log.V(0).Info("Index created",
		"index", indexName)

	// TODO: apply and roll back all changes transactionally
	if _, err := kutils.KubeApply(r, ctx, secret); err != nil {
		log.Error(err, "Unable to apply Secret",
			"name", secret.Name,
			"namespace", secret.Namespace)
		return ctrl.Result{}, err
	}
	log.V(0).Info("Secret applied",
		"name", secret.Name,
		"namespace", secret.Namespace)

	// Update status of Elasticsearch object
	esObj.Status.Ready = true
	if err := r.Status().Update(ctx, esObj); err != nil {
		log.Error(err, "unable to update Elasticsearch status",
			"name", esObj.Name,
			"namespace", esObj.Namespace)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ElasticsearchReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&jangofettv1alpha1.Elasticsearch{}).
		Complete(r)
}
