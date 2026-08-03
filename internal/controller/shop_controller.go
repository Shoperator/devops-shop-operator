package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	shopv1 "github.com/slepimis120/devops-shop-operator/api/v1"
)

// shopFinalizerName is the finalizer used for Shop resources.
// const shopFinalizerName = "shop.shophub.local/finalizer"

// ShopReconciler reconciles a Shop object
type ShopReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=shop.shophub.local,resources=shops,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=shop.shophub.local,resources=shops/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=shop.shophub.local,resources=shops/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ShopReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Shop resource PRVO
	shop := &shopv1.Shop{}
	if err := r.Get(ctx, req.NamespacedName, shop); err != nil {
		log.Error(err, "unable to fetch Shop")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if shop.Spec.Image == "" {
		shop.Spec.Image = "nginx:1.24" // Default
	}

	// Determine number of replicas based on availability
	var replicas int32 = 2
	if shop.Spec.Availability == "high" {
		replicas = 3
	}

	// Create Deployment
	deployment := &appsv1.Deployment{}
	deploymentName := types.NamespacedName{
		Name:      fmt.Sprintf("%s-deployment", shop.Name),
		Namespace: shop.Namespace,
	}

	err := r.Get(ctx, deploymentName, deployment)
	if err != nil && client.IgnoreNotFound(err) != nil {
		log.Error(err, "unable to fetch Deployment")
		return ctrl.Result{}, err
	}

	if err != nil {
		// Deployment doesn't exist, create it
		deployment = constructDeployment(shop, replicas)
		if err := controllerutil.SetControllerReference(shop, deployment, r.Scheme); err != nil {
			log.Error(err, "unable to set owner reference on new Deployment")
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, deployment); err != nil {
			log.Error(err, "unable to create new Deployment", "Deployment", deployment)
			return ctrl.Result{}, err
		}
		log.Info("Created new Deployment", "Deployment", deploymentName)
	} else {
		// Deployment exists, update if needed
		deployment.Spec.Replicas = &replicas
		if err := r.Update(ctx, deployment); err != nil {
			log.Error(err, "unable to update Deployment", "Deployment", deploymentName)
			return ctrl.Result{}, err
		}
		log.Info("Updated Deployment replicas", "Replicas", replicas)
	}

	// Create Service
	service := &corev1.Service{}
	serviceName := types.NamespacedName{
		Name:      fmt.Sprintf("%s-service", shop.Name),
		Namespace: shop.Namespace,
	}

	err = r.Get(ctx, serviceName, service)
	if err != nil && client.IgnoreNotFound(err) != nil {
		log.Error(err, "unable to fetch Service")
		return ctrl.Result{}, err
	}

	if err != nil {
		// Service doesn't exist, create it
		service = constructService(shop)
		if err := controllerutil.SetControllerReference(shop, service, r.Scheme); err != nil {
			log.Error(err, "unable to set owner reference on new Service")
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, service); err != nil {
			log.Error(err, "unable to create new Service", "Service", service)
			return ctrl.Result{}, err
		}
		log.Info("Created new Service", "Service", serviceName)
	}

	desiredURL := fmt.Sprintf("http://%s-service.%s.svc.cluster.local", shop.Name, shop.Namespace)
	if shop.Status.Status != "Running" || shop.Status.Replicas != replicas || shop.Status.URL != desiredURL {
		shop.Status.Status = "Running"
		shop.Status.Replicas = replicas
		shop.Status.URL = desiredURL
		if err := r.Status().Update(ctx, shop); err != nil {
			log.Error(err, "unable to update Shop status")
			return ctrl.Result{}, err
		}
		log.Info("Updated Shop status", "Shop", shop.Name)
	}

	log.Info("Successfully reconciled Shop", "Shop", shop.Name)
	return ctrl.Result{}, nil
}

// constructDeployment constructs a Deployment for the given Shop
func constructDeployment(shop *shopv1.Shop, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-deployment", shop.Name),
			Namespace: shop.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": shop.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": shop.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "shop",
							Image: shop.Spec.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 80,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "SHOP_NAME",
									Value: shop.Spec.Name,
								},
								{
									Name:  "WALLET_ADDRESS",
									Value: shop.Spec.WalletAddress,
								},
								{
									Name:  "DATABASE_TYPE",
									Value: shop.Spec.Database,
								},
							},
						},
					},
				},
			},
		},
	}
}

// constructService constructs a Service for the given Shop
func constructService(shop *shopv1.Shop) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-service", shop.Name),
			Namespace: shop.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": shop.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(80),
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ShopReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&shopv1.Shop{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
