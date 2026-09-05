package controller

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	shopv1 "github.com/slepimis120/devops-shop-operator/api/v1"
)

// Browsers resolve every name under `localhost` to the loopback address by
// themselves, so a shop created through ShopHub is reachable the moment it is
// deployed
const DefaultBaseDomain = "localhost"

// ShopReconciler reconciles a Shop object
type ShopReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// BaseDomain is the suffix every shop is published under: a Shop named
	// "shop1" is served at "shop1.<BaseDomain>".
	//
	// This is the only place the operator knows about the outside world's
	// addressing. ShopHub has to be told the same domain, since it builds the
	// link available in dashboard.
	BaseDomain string
}

func (r *ShopReconciler) baseDomain() string {
	if r.BaseDomain == "" {
		return DefaultBaseDomain
	}
	return r.BaseDomain
}

//+kubebuilder:rbac:groups=shop.shophub.local,resources=shops,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=shop.shophub.local,resources=shops/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=shop.shophub.local,resources=shops/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=postgresql.cnpg.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete

func (r *ShopReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	shop := &shopv1.Shop{}
	if err := r.Get(ctx, req.NamespacedName, shop); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var replicas int32 = 2
	if shop.Spec.Availability == "high" {
		replicas = 3
	}

	if err := r.reconcileDatabase(ctx, shop); err != nil {
		logger.Error(err, "unable to reconcile database")
		return ctrl.Result{}, err
	}
	if err := r.reconcileAuthSecret(ctx, shop); err != nil {
		logger.Error(err, "unable to reconcile auth secret")
		return ctrl.Result{}, err
	}

	// backend
	if err := r.ensureDeployment(ctx, shop, constructBackendDeployment(shop, replicas), replicas); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureCreated(ctx, shop, constructService(shop, "backend")); err != nil {
		return ctrl.Result{}, err
	}
	// frontend
	if err := r.ensureDeployment(ctx, shop, constructFrontendDeployment(shop, replicas), replicas); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.ensureCreated(ctx, shop, constructService(shop, "frontend")); err != nil {
		return ctrl.Result{}, err
	}
	// ingress
	domain := r.baseDomain()
	if err := r.ensureIngress(ctx, shop, constructIngress(shop, domain)); err != nil {
		return ctrl.Result{}, err
	}

	desiredURL := fmt.Sprintf("http://%s.%s", shop.Name, domain)
	if shop.Status.Status != "Running" || shop.Status.Replicas != replicas || shop.Status.URL != desiredURL {
		shop.Status.Status = "Running"
		shop.Status.Replicas = replicas
		shop.Status.URL = desiredURL
		if err := r.Status().Update(ctx, shop); err != nil {
			return ctrl.Result{}, err
		}
	}
	logger.Info("Successfully reconciled Shop", "Shop", shop.Name)
	return ctrl.Result{}, nil
}

// ---- helpers ----

func (r *ShopReconciler) ensureCreated(ctx context.Context, shop *shopv1.Shop, obj client.Object) error {
	check := obj.DeepCopyObject().(client.Object)
	err := r.Get(ctx, client.ObjectKeyFromObject(obj), check)
	if err == nil {
		return nil
	}
	if client.IgnoreNotFound(err) != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(shop, obj, r.Scheme); err != nil {
		return err
	}
	return r.Create(ctx, obj)
}

// ensureDeployment creates the Deployment, or brings a running one back in line
// with the Shop it belongs to.
//
// Only the three things the Shop's spec actually feeds are compared and copied:
// the replica count, the image, and the environment. Replacing the whole pod
// template instead would fight the defaults the API server fills in — they are
// absent from the desired template, so every comparison would differ and every
// reconcile would write again.
func (r *ShopReconciler) ensureDeployment(ctx context.Context, shop *shopv1.Shop, desired *appsv1.Deployment, replicas int32) error {
	found := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), found)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return err
		}
		if err := controllerutil.SetControllerReference(shop, desired, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, desired)
	}

	if len(found.Spec.Template.Spec.Containers) == 0 || len(desired.Spec.Template.Spec.Containers) == 0 {
		return fmt.Errorf("deployment %s has no container to reconcile", found.Name)
	}
	current := &found.Spec.Template.Spec.Containers[0]
	wanted := desired.Spec.Template.Spec.Containers[0]
	changed := false

	if found.Spec.Replicas == nil || *found.Spec.Replicas != replicas {
		found.Spec.Replicas = &replicas
		changed = true
	}
	if current.Image != wanted.Image {
		current.Image = wanted.Image
		changed = true
	}
	// The environment carries the wallet address and the admin username, so
	// this is what makes reconfiguring a shop reach the pods that are running.
	if !equality.Semantic.DeepEqual(current.Env, wanted.Env) {
		current.Env = wanted.Env
		changed = true
	}

	if !changed {
		return nil
	}
	log.FromContext(ctx).Info("Updating Deployment to match the Shop", "Deployment", found.Name)
	return r.Update(ctx, found)
}

// ensureIngress creates the Ingress that publishes the shop
func (r *ShopReconciler) ensureIngress(ctx context.Context, shop *shopv1.Shop, desired *networkingv1.Ingress) error {
	found := &networkingv1.Ingress{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), found)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return err
		}
		if err := controllerutil.SetControllerReference(shop, desired, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, desired)
	}

	if equality.Semantic.DeepEqual(found.Spec.Rules, desired.Spec.Rules) &&
		equality.Semantic.DeepEqual(found.Spec.IngressClassName, desired.Spec.IngressClassName) {
		return nil
	}

	found.Spec.Rules = desired.Spec.Rules
	found.Spec.IngressClassName = desired.Spec.IngressClassName
	log.FromContext(ctx).Info("Updating Ingress to match the Shop", "Ingress", found.Name)
	return r.Update(ctx, found)
}

func (r *ShopReconciler) reconcileDatabase(ctx context.Context, shop *shopv1.Shop) error {
	logger := log.FromContext(ctx)
	if shop.Spec.Database != "postgresql" {
		return nil
	}
	name := fmt.Sprintf("%s-db", shop.Name)
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(schema.GroupVersionKind{Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster"})
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: shop.Namespace}, cluster)
	if err == nil {
		return nil
	}
	if meta.IsNoMatchError(err) {
		logger.Info("CNPG CRD nije instaliran, preskačem DB provisioning", "shop", shop.Name)
		return nil
	}
	if client.IgnoreNotFound(err) != nil {
		return err
	}
	cluster.SetName(name)
	cluster.SetNamespace(shop.Namespace)
	cluster.Object["spec"] = map[string]interface{}{
		"instances": int64(1),
		"storage":   map[string]interface{}{"size": "1Gi"},
	}
	if err := controllerutil.SetControllerReference(shop, cluster, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, cluster); err != nil {
		if meta.IsNoMatchError(err) {
			return nil
		}
		return err
	}
	return nil
}

func (r *ShopReconciler) reconcileAuthSecret(ctx context.Context, shop *shopv1.Shop) error {
	name := fmt.Sprintf("%s-auth", shop.Name)
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: shop.Namespace}, existing)
	if err == nil {
		return nil
	}
	if client.IgnoreNotFound(err) != nil {
		return err
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: shop.Namespace},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"JWT_SECRET":          randString(32),
			"SHOP_ADMIN_PASSWORD": randString(16),
		},
	}
	if err := controllerutil.SetControllerReference(shop, secret, r.Scheme); err != nil {
		return err
	}
	return r.Create(ctx, secret)
}

func randString(n int) string {
	b := make([]byte, n)
	_, _ = crand.Read(b)
	return hex.EncodeToString(b)
}

func secretKeyRef(secret, key string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: secret},
			Key:                  key,
		},
	}
}

// ---- construct ----

func deploymentFor(shop *shopv1.Shop, name, image string, replicas int32, labels map[string]string, env []corev1.EnvVar) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: shop.Namespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "app",
						Image: image,
						Ports: []corev1.ContainerPort{{ContainerPort: 3000}},
						Env:   env,
					}},
				},
			},
		},
	}
}

func constructBackendDeployment(shop *shopv1.Shop, replicas int32) *appsv1.Deployment {
	name := fmt.Sprintf("%s-backend", shop.Name)
	auth := fmt.Sprintf("%s-auth", shop.Name)
	env := []corev1.EnvVar{
		{Name: "PORT", Value: "3000"},
		{Name: "WALLET_ADDRESS", Value: shop.Spec.WalletAddress},
		{Name: "SHOP_ADMIN_USERNAME", Value: shop.Spec.AdminUsername},
		{Name: "SHOP_ADMIN_PASSWORD", ValueFrom: secretKeyRef(auth, "SHOP_ADMIN_PASSWORD")},
		{Name: "JWT_SECRET", ValueFrom: secretKeyRef(auth, "JWT_SECRET")},
	}
	if shop.Spec.Database == "postgresql" {
		s := fmt.Sprintf("%s-db-app", shop.Name)
		env = append(env,
			corev1.EnvVar{Name: "DB_HOST", ValueFrom: secretKeyRef(s, "host")},
			corev1.EnvVar{Name: "DB_PORT", ValueFrom: secretKeyRef(s, "port")},
			corev1.EnvVar{Name: "DB_USERNAME", ValueFrom: secretKeyRef(s, "username")},
			corev1.EnvVar{Name: "DB_PASSWORD", ValueFrom: secretKeyRef(s, "password")},
			corev1.EnvVar{Name: "DB_NAME", ValueFrom: secretKeyRef(s, "dbname")},
		)
	}
	return deploymentFor(shop, name, shop.Spec.BackendImage, replicas, map[string]string{"app": name}, env)
}

// constructFrontendDeployment builds the shop's storefront. 
// Note the address of the backend is not here.
//
// The Ingress publishes both halves of a shop under one host, so the storefront
// addresses the backend with path-only URLs `/api/v1/articles` which the
// browser resolves against whatever host the customer reached the shop on.
func constructFrontendDeployment(shop *shopv1.Shop, replicas int32) *appsv1.Deployment {
	name := fmt.Sprintf("%s-frontend", shop.Name)
	env := []corev1.EnvVar{
		{Name: "PORT", Value: "3000"},
		{Name: "HOSTNAME", Value: "0.0.0.0"},
		{Name: "NEXT_PUBLIC_SHOP_NAME", Value: shop.Spec.Name},
	}
	return deploymentFor(shop, name, shop.Spec.FrontendImage, replicas, map[string]string{"app": name}, env)
}

func constructService(shop *shopv1.Shop, component string) *corev1.Service {
	name := fmt.Sprintf("%s-%s", shop.Name, component)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: shop.Namespace},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports:    []corev1.ServicePort{{Port: 3000, TargetPort: intstr.FromInt(3000)}},
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
}

func ingressBackend(svc string, port int32) networkingv1.IngressBackend {
	return networkingv1.IngressBackend{
		Service: &networkingv1.IngressServiceBackend{
			Name: svc,
			Port: networkingv1.ServiceBackendPort{Number: port},
		},
	}
}

// constructIngress publishes the whole shop under one host
// `/api` goes to the backend, 
// everything else to the storefront. 
// nginx matches the longest path first, so the two paths do not compete
//
// One host for both halves is what makes the storefront's requests same-origin,
// so no CORS is involved and the storefront needs no address of its own.
func constructIngress(shop *shopv1.Shop, baseDomain string) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	ingressClass := "nginx"
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-ingress", shop.Name), Namespace: shop.Namespace},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingressClass,
			Rules: []networkingv1.IngressRule{{
				Host: fmt.Sprintf("%s.%s", shop.Name, baseDomain),
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{Path: "/api", PathType: &pathType, Backend: ingressBackend(fmt.Sprintf("%s-backend", shop.Name), 3000)},
							{Path: "/", PathType: &pathType, Backend: ingressBackend(fmt.Sprintf("%s-frontend", shop.Name), 3000)},
						},
					},
				},
			}},
		},
	}
}

func (r *ShopReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&shopv1.Shop{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&networkingv1.Ingress{}).
		Complete(r)
}
