package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	shopv1 "github.com/slepimis120/devops-shop-operator/api/v1"
)

// DiscordChannelReconciler reconciles a DiscordChannel object
type DiscordChannelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=shop.shophub.local,resources=discordchannels,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=shop.shophub.local,resources=discordchannels/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=shop.shophub.local,resources=discordchannels/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *DiscordChannelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the DiscordChannel resource
	discordChannel := &shopv1.DiscordChannel{}
	if err := r.Get(ctx, req.NamespacedName, discordChannel); err != nil {
		log.Error(err, "unable to fetch DiscordChannel")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// For now, just mark as configured
	discordChannel.Status.Status = "Configured"
	discordChannel.Status.ChannelID = fmt.Sprintf("channel-%s", discordChannel.Name)

	if err := r.Status().Update(ctx, discordChannel); err != nil {
		log.Error(err, "unable to update DiscordChannel status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled DiscordChannel", "DiscordChannel", discordChannel.Name)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DiscordChannelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&shopv1.DiscordChannel{}).
		Complete(r)
}
