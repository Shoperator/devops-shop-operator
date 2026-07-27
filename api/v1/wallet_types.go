package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WalletSpec struct {
	// Wallet address
	Address string `json:"address"`

	// Blockchain network: "ethereum", "bitcoin", "polygon"
	Network string `json:"network"`

	// Private key (stored as secret reference)
	PrivateKeySecretRef string `json:"privateKeySecretRef,omitempty"`
}

type WalletStatus struct {
	// Status: "Active", "Invalid"
	Status string `json:"status,omitempty"`

	// Balance (if applicable)
	Balance string `json:"balance,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

type Wallet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WalletSpec   `json:"spec,omitempty"`
	Status WalletStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

type WalletList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Wallet `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Wallet{}, &WalletList{})
}
