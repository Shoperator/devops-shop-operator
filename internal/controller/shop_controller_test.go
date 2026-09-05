/*
Copyright 2026.

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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	shopv1 "github.com/slepimis120/devops-shop-operator/api/v1"
)

var _ = Describe("Shop Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		shop := &shopv1.Shop{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Shop")
			err := k8sClient.Get(ctx, typeNamespacedName, shop)
			if err != nil && errors.IsNotFound(err) {
				resource := &shopv1.Shop{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: shopv1.ShopSpec{
						Name:          "Test Shop",
						Availability:  "standard",
						WalletAddress: "0x0000000000000000000000000000000000000000",
						Database:      "postgresql",
						BackendImage:  "slepimis120/devops-shop-backend:0.1.0",
						FrontendImage: "slepimis120/devops-shop-frontend:0.1.0",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &shopv1.Shop{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Shop")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ShopReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})

// envValueOf reads a plain environment value off a container, which is where
// the settings an owner can change end up.
func envValueOf(container corev1.Container, name string) string {
	for _, env := range container.Env {
		if env.Name == name {
			return env.Value
		}
	}
	return ""
}

// declaresEnv reports whether the container carries the variable at all.
//
// Distinct from envValueOf on purpose: "the storefront is told no backend
// address" has to mean the variable is absent, since an empty value is still a
// value the app would read.
func declaresEnv(container corev1.Container, name string) bool {
	for _, env := range container.Env {
		if env.Name == name {
			return true
		}
	}
	return false
}

var _ = Describe("Shop Controller reconfiguring a deployed shop", func() {
	const resourceName = "reconfigure-shop"
	const oldWallet = "0x0000000000000000000000000000000000000001"
	const newWallet = "0x0000000000000000000000000000000000000002"

	ctx := context.Background()

	shopName := types.NamespacedName{Name: resourceName, Namespace: "default"}
	backendName := types.NamespacedName{Name: resourceName + "-backend", Namespace: "default"}

	reconciler := func() *ShopReconciler {
		return &ShopReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
	}

	reconcileShop := func() {
		_, err := reconciler().Reconcile(ctx, reconcile.Request{NamespacedName: shopName})
		Expect(err).NotTo(HaveOccurred())
	}

	backend := func() *appsv1.Deployment {
		deployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, backendName, deployment)).To(Succeed())
		return deployment
	}

	BeforeEach(func() {
		resource := &shopv1.Shop{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
			Spec: shopv1.ShopSpec{
				Name:          "Prodavnica odece",
				Availability:  "standard",
				WalletAddress: oldWallet,
				Database:      "postgresql",
				BackendImage:  "slepimis120/devops-shop-backend:0.1.0",
				FrontendImage: "slepimis120/devops-shop-frontend:0.1.0",
				AdminUsername: "admin",
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		reconcileShop()
	})

	AfterEach(func() {
		// Nothing garbage-collects owner references in envtest, so the children
		// have to go by hand or they outlive the Shop and skew the next test.
		shop := &shopv1.Shop{}
		Expect(k8sClient.Get(ctx, shopName, shop)).To(Succeed())
		Expect(k8sClient.Delete(ctx, shop)).To(Succeed())

		for _, component := range []string{"-backend", "-frontend"} {
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName + component, Namespace: "default"},
			}
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, deployment))).To(Succeed())
		}
	})

	It("deploys the shop with the settings it was created with", func() {
		Expect(envValueOf(backend().Spec.Template.Spec.Containers[0], "WALLET_ADDRESS")).To(Equal(oldWallet))
		Expect(*backend().Spec.Replicas).To(Equal(int32(2)))
	})

	It("moves the payments to a new wallet on the running pods", func() {
		shop := &shopv1.Shop{}
		Expect(k8sClient.Get(ctx, shopName, shop)).To(Succeed())
		shop.Spec.WalletAddress = newWallet
		Expect(k8sClient.Update(ctx, shop)).To(Succeed())

		reconcileShop()

		Expect(envValueOf(backend().Spec.Template.Spec.Containers[0], "WALLET_ADDRESS")).To(Equal(newWallet))
	})

	It("scales the shop when its availability changes", func() {
		shop := &shopv1.Shop{}
		Expect(k8sClient.Get(ctx, shopName, shop)).To(Succeed())
		shop.Spec.Availability = "high"
		Expect(k8sClient.Update(ctx, shop)).To(Succeed())

		reconcileShop()

		Expect(*backend().Spec.Replicas).To(Equal(int32(3)))
	})

	It("rolls out a new image", func() {
		shop := &shopv1.Shop{}
		Expect(k8sClient.Get(ctx, shopName, shop)).To(Succeed())
		shop.Spec.BackendImage = "slepimis120/devops-shop-backend:0.2.0"
		Expect(k8sClient.Update(ctx, shop)).To(Succeed())

		reconcileShop()

		Expect(backend().Spec.Template.Spec.Containers[0].Image).To(Equal("slepimis120/devops-shop-backend:0.2.0"))
	})

	It("writes nothing when nothing changed", func() {
		before := backend().ResourceVersion

		reconcileShop()

		Expect(backend().ResourceVersion).To(Equal(before))
	})
})

var _ = Describe("Shop Controller publishing a shop", func() {
	const resourceName = "published-shop"

	ctx := context.Background()

	shopName := types.NamespacedName{Name: resourceName, Namespace: "default"}
	ingressName := types.NamespacedName{Name: resourceName + "-ingress", Namespace: "default"}
	frontendName := types.NamespacedName{Name: resourceName + "-frontend", Namespace: "default"}

	reconcileUnder := func(baseDomain string) {
		reconciler := &ShopReconciler{
			Client:     k8sClient,
			Scheme:     k8sClient.Scheme(),
			BaseDomain: baseDomain,
		}
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: shopName})
		Expect(err).NotTo(HaveOccurred())
	}

	ingress := func() *networkingv1.Ingress {
		published := &networkingv1.Ingress{}
		Expect(k8sClient.Get(ctx, ingressName, published)).To(Succeed())
		return published
	}

	frontend := func() corev1.Container {
		deployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, frontendName, deployment)).To(Succeed())
		return deployment.Spec.Template.Spec.Containers[0]
	}

	BeforeEach(func() {
		resource := &shopv1.Shop{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: "default"},
			Spec: shopv1.ShopSpec{
				Name:          "Prodavnica odece",
				Availability:  "standard",
				WalletAddress: "0x0000000000000000000000000000000000000003",
				Database:      "postgresql",
				BackendImage:  "slepimis120/devops-shop-backend:0.1.0",
				FrontendImage: "slepimis120/devops-shop-frontend:0.1.0",
			},
		}
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
	})

	AfterEach(func() {
		// Nothing garbage-collects owner references in envtest, so a leftover
		// Ingress would be reconciled instead of created by the next test.
		shop := &shopv1.Shop{}
		Expect(k8sClient.Get(ctx, shopName, shop)).To(Succeed())
		Expect(k8sClient.Delete(ctx, shop)).To(Succeed())

		published := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: ingressName.Name, Namespace: ingressName.Namespace},
		}
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, published))).To(Succeed())

		for _, component := range []string{"-backend", "-frontend"} {
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: resourceName + component, Namespace: "default"},
			}
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, deployment))).To(Succeed())
		}
	})

	It("publishes the shop under a host built from the base domain", func() {
		reconcileUnder("localhost")

		Expect(ingress().Spec.Rules).To(HaveLen(1))
		Expect(ingress().Spec.Rules[0].Host).To(Equal("published-shop.localhost"))
	})

	It("falls back to the default domain when the operator was given none", func() {
		reconcileUnder("")

		Expect(ingress().Spec.Rules[0].Host).To(Equal("published-shop." + DefaultBaseDomain))
	})

	It("sends /api to the backend and everything else to the storefront", func() {
		reconcileUnder("localhost")

		paths := ingress().Spec.Rules[0].HTTP.Paths
		Expect(paths).To(HaveLen(2))
		Expect(paths[0].Path).To(Equal("/api"))
		Expect(paths[0].Backend.Service.Name).To(Equal(resourceName + "-backend"))
		Expect(paths[1].Path).To(Equal("/"))
		Expect(paths[1].Backend.Service.Name).To(Equal(resourceName + "-frontend"))
	})

	// Both halves answering on one host is what lets the storefront address the
	// backend relative to the page it is on, so it is told the shop's identity
	// and nothing about where anything lives.
	It("tells the storefront nothing about where the backend is", func() {
		reconcileUnder("localhost")

		Expect(declaresEnv(frontend(), "NEXT_PUBLIC_API_URL")).To(BeFalse())
		Expect(envValueOf(frontend(), "NEXT_PUBLIC_SHOP_NAME")).To(Equal("Prodavnica odece"))
	})

	// The domain is a property of the cluster, not of the shop, so changing it
	// has to move the shops that are already deployed.
	It("moves a shop already in the cluster to a new base domain", func() {
		reconcileUnder("localhost")

		reconcileUnder("127.0.0.1.sslip.io")

		Expect(ingress().Spec.Rules[0].Host).To(Equal("published-shop.127.0.0.1.sslip.io"))
	})

	It("leaves the Ingress alone when the domain has not changed", func() {
		reconcileUnder("localhost")
		before := ingress().ResourceVersion

		reconcileUnder("localhost")

		Expect(ingress().ResourceVersion).To(Equal(before))
	})

	It("records the URL the shop is reachable at", func() {
		reconcileUnder("localhost")

		shop := &shopv1.Shop{}
		Expect(k8sClient.Get(ctx, shopName, shop)).To(Succeed())
		Expect(shop.Status.URL).To(Equal("http://published-shop.localhost"))
	})
})
