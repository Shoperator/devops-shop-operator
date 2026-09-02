package controller

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"golang.org/x/crypto/sha3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create

func (r *WalletReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	wallet := &shopv1.Wallet{}
	if err := r.Get(ctx, req.NamespacedName, wallet); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	network := wallet.Spec.Network
	if network == "" {
		network = "ethereum"
	}
	// Only EVM networks share the secp256k1/keccak address format.
	if network != "ethereum" && network != "polygon" {
		logger.Info("unsupported network for key generation", "network", network)
		wallet.Status.Phase = "Unsupported"
		wallet.Status.Address = ""
		return ctrl.Result{}, r.Status().Update(ctx, wallet)
	}

	secretName := fmt.Sprintf("%s-wallet", wallet.Name)
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: wallet.Namespace}, secret)
	if err == nil {
		// Already exists — read the address back from the Secret, don't regenerate (idempotent).
		return r.updateStatus(ctx, wallet, string(secret.Data["ADDRESS"]), secretName)
	}
	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	// Generate a new secp256k1 keypair and derive the EVM address.
	priv, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return ctrl.Result{}, err
	}
	privHex := hex.EncodeToString(priv.Serialize())
	address := ethAddressFromPubKey(priv.PubKey())

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: wallet.Namespace},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"PRIVATE_KEY": privHex,
			"ADDRESS":     address,
			"NETWORK":     network,
		},
	}
	if err := controllerutil.SetControllerReference(wallet, newSecret, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Create(ctx, newSecret); err != nil {
		return ctrl.Result{}, err
	}
	logger.Info("generated wallet keypair", "wallet", wallet.Name, "address", address)

	return r.updateStatus(ctx, wallet, address, secretName)
}

// ethAddressFromPubKey derives an Ethereum address: keccak256 over the uncompressed
// public key (without the 0x04 prefix), taking the last 20 bytes. Returns a lowercase
// 0x address (no EIP-55 checksum casing; Metamask accepts it either way).
func ethAddressFromPubKey(pub *secp256k1.PublicKey) string {
	raw := pub.SerializeUncompressed() // 65 bytes: 0x04 || X || Y
	h := sha3.NewLegacyKeccak256()
	h.Write(raw[1:])
	sum := h.Sum(nil)
	return "0x" + hex.EncodeToString(sum[12:])
}

func (r *WalletReconciler) updateStatus(ctx context.Context, wallet *shopv1.Wallet, address, secretName string) (ctrl.Result, error) {
	balance := wallet.Status.Balance
	if balance == "" {
		balance = "0"
	}
	// Skip the write when the status is already what we want; otherwise the
	// Secret-triggered reconciles would keep writing.
	if wallet.Status.Phase == "Active" &&
		wallet.Status.Address == address &&
		wallet.Status.SecretRef == secretName &&
		wallet.Status.Balance == balance {
		return ctrl.Result{}, nil
	}
	// MergeFrom patch carries no resourceVersion, so a slightly stale cache
	// read can't cause an optimistic-lock conflict on the status write.
	patch := client.MergeFrom(wallet.DeepCopy())
	wallet.Status.Phase = "Active"
	wallet.Status.Address = address
	wallet.Status.SecretRef = secretName
	wallet.Status.Balance = balance
	return ctrl.Result{}, r.Status().Patch(ctx, wallet, patch)
}

// SetupWithManager sets up the controller with the Manager.
func (r *WalletReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&shopv1.Wallet{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
