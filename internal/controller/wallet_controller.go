package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	shopv1 "github.com/slepimis120/devops-shop-operator/api/v1"
)

// WalletReconciler reconciles a Wallet object
type WalletReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=shop.shophub.local,resources=wallets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=shop.shophub.local,resources=wallets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=shop.shophub.local,resources=wallets/finalizers,verbs=update

func (r *WalletReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the Wallet resource
	wallet := &shopv1.Wallet{}
	if err := r.Get(ctx, req.NamespacedName, wallet); err != nil {
		log.Error(err, "unable to fetch Wallet")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Mark wallet as active (placeholder - kasnije se konetkuje na blockchain)
	wallet.Status.Status = "Active"
	wallet.Status.Balance = "0"

	if err := r.Status().Update(ctx, wallet); err != nil {
		log.Error(err, "unable to update Wallet status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled Wallet", "Wallet", wallet.Name)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WalletReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&shopv1.Wallet{}).
		Complete(r)
}
