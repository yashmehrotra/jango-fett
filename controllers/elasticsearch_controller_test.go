package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/olivere/elastic/v7"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	jangofettv1alpha1 "gitlab.com/beecash/infra/jango-fett/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// To understand these tests, read past the initializations and focus on the test specs
var _ = Describe("Elasticsearch controller", func() {
	ctx := context.Background()

	const (
		EsUrl              = "http://localhost:9200"
		esObjectName       = "es"
		k8sNs              = "default" // changing this means that we will have to create the ns as a part of test setup
		expectedIndexName  = "jf-default-es"
		expectedSecretName = "jf-elasticsearch-" + esObjectName
		// timeout and retry intervals while waiting on k8s operations
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	esObj := &jangofettv1alpha1.Elasticsearch{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "jango-fett.bukukas.io/v1alpha1",
			Kind:       "Elasticsearch",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      esObjectName,
			Namespace: k8sNs,
		},
		Spec: jangofettv1alpha1.ElasticsearchSpec{},
	}

	secretLookupKey := types.NamespacedName{Name: expectedSecretName, Namespace: k8sNs}

	var es, err = elastic.NewClient(elastic.SetURL(EsUrl), elastic.SetSniff(false))
	if err != nil {
		panic(err)
	}

	Context("CR creation", func() {
		It("Should accept the elasticsearch CR", func() {
			Expect(k8sClient.Create(ctx, esObj)).Should(Succeed())
		})

		It("should create the corresponding index in elasticsearch", func() {
			Eventually(func() (bool, error) {
				return es.IndexExists(expectedIndexName).Do(ctx)
			}, timeout, interval).Should(BeTrue())
		})

		It("should create the corresponding secret with connection params", func() {
			secret := &corev1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(ctx, secretLookupKey, secret)
			}, timeout, interval).Should(Succeed())

			Expect(string(secret.Data["ELASTICSEARCH_URL"])).Should(Equal(EsUrl))
			Expect(string(secret.Data["ES_INDEX_NAME"])).Should(Equal(expectedIndexName))
		})

		It("should check the ready status of CR", func() {
			Eventually(func() bool {
				updatedESObj := &jangofettv1alpha1.Elasticsearch{}
				updatedESObjLookupKey := types.NamespacedName{
					Name:      esObjectName,
					Namespace: k8sNs,
				}
				err := k8sClient.Get(ctx, updatedESObjLookupKey, updatedESObj)
				if err != nil {
					fmt.Println(err)
					return false
				}
				return updatedESObj.Status.Ready
			}, timeout, interval).Should(BeTrue())

		})

		It("should handle CR deletion", func() {
			Eventually(func() error {
				return k8sClient.Delete(ctx, esObj)
			}, timeout, interval).Should(Succeed())
		})

		It("should delete the corresponding index in elasticsearch", func() {
			Eventually(func() (bool, error) {
				return es.IndexExists(expectedIndexName).Do(ctx)
			}, timeout, interval).Should(BeFalse())
		})
		It("should delete the corresponding secret", func() {
			secret := &corev1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(ctx, secretLookupKey, secret)
			}, timeout, interval).ShouldNot(Succeed())
		})
	})
})
