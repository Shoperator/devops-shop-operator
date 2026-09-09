package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	shopv1 "github.com/slepimis120/devops-shop-operator/api/v1"
)

// Key inside the referenced Secret that holds the Discord bot token.
const discordBotTokenKey = "token"

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
	logger := log.FromContext(ctx)

	dc := &shopv1.DiscordChannel{}
	if err := r.Get(ctx, req.NamespacedName, dc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Already done — don't call the Discord API again on every event.
	if dc.Status.Status == "Configured" && dc.Status.ChannelID != "" {
		return ctrl.Result{}, nil
	}

	token, err := r.botToken(ctx, dc)
	if err != nil {
		return r.fail(ctx, dc, err)
	}

	// REST-only usage: discordgo issues API calls with the bot token without
	// opening a gateway connection.
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return r.fail(ctx, dc, fmt.Errorf("discord session: %w", err))
	}

	channelID, err := ensureDiscordChannel(session, dc.Spec.ServerID, dc.Spec.ChannelName)
	if err != nil {
		return r.fail(ctx, dc, err)
	}

	dc.Status.Status = "Configured"
	dc.Status.ChannelID = channelID
	logger.Info("reconciled DiscordChannel", "name", dc.Name, "channelID", channelID)
	return ctrl.Result{}, r.Status().Update(ctx, dc)
}

// botToken reads the bot token from the Secret named in the spec (same namespace).
func (r *DiscordChannelReconciler) botToken(ctx context.Context, dc *shopv1.DiscordChannel) (string, error) {
	if dc.Spec.BotTokenSecretRef == "" {
		return "", fmt.Errorf("spec.botTokenSecretRef is empty")
	}
	secret := &corev1.Secret{}
	name := types.NamespacedName{Name: dc.Spec.BotTokenSecretRef, Namespace: dc.Namespace}
	if err := r.Get(ctx, name, secret); err != nil {
		return "", fmt.Errorf("read bot token secret %q: %w", dc.Spec.BotTokenSecretRef, err)
	}
	token := string(secret.Data[discordBotTokenKey])
	if token == "" {
		return "", fmt.Errorf("secret %q has no %q key", dc.Spec.BotTokenSecretRef, discordBotTokenKey)
	}
	return token, nil
}

// ensureDiscordChannel returns the ID of the text channel named channelName in
// the guild, creating it if missing (idempotent by name).
func ensureDiscordChannel(session *discordgo.Session, guildID, channelName string) (string, error) {
	if guildID == "" || channelName == "" {
		return "", fmt.Errorf("serverId and channelName are required")
	}
	channels, err := session.GuildChannels(guildID)
	if err != nil {
		return "", fmt.Errorf("list guild channels: %w", err)
	}
	for _, ch := range channels {
		if ch.Type == discordgo.ChannelTypeGuildText && ch.Name == channelName {
			return ch.ID, nil
		}
	}
	created, err := session.GuildChannelCreate(guildID, channelName, discordgo.ChannelTypeGuildText)
	if err != nil {
		return "", fmt.Errorf("create channel: %w", err)
	}
	return created.ID, nil
}

// fail records the failure in status and retries later, without hard-failing
// the work queue on transient Discord API problems.
func (r *DiscordChannelReconciler) fail(ctx context.Context, dc *shopv1.DiscordChannel, err error) (ctrl.Result, error) {
	log.FromContext(ctx).Error(err, "DiscordChannel reconcile failed", "name", dc.Name)
	dc.Status.Status = "Failed"
	if uerr := r.Status().Update(ctx, dc); uerr != nil {
		return ctrl.Result{}, uerr
	}
	return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DiscordChannelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&shopv1.DiscordChannel{}).
		Complete(r)
}
