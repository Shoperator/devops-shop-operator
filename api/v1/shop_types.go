package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ShopSpec defines the desired state of Shop
type ShopSpec struct {
	// Name of the shop
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Availability: "standard" (2 replicas) or "high" (3 replicas)
	// +kubebuilder:validation:Enum=standard;high
	// +kubebuilder:validation:Required
	Availability string `json:"availability"`

	// Wallet address for payments
	// +kubebuilder:validation:Required
	WalletAddress string `json:"walletAddress"`

	// Database type: "postgresql" or "redis"
	// +kubebuilder:validation:Enum=postgresql;redis
	// +kubebuilder:validation:Required
	Database string `json:"database"`

	// +kubebuilder:validation:Optional
	// +kubebuilder:default="nginx:1.24"
	Image string `json:"image,omitempty"`
}

// ShopStatus defines the observed state of Shop
type ShopStatus struct {
	// Status of the deployment: "Pending", "Running", "Failed"
	Status string `json:"status,omitempty"`

	// URL where the shop is accessible
	URL string `json:"url,omitempty"`

	// Number of replicas
	Replicas int32 `json:"replicas,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.name`
//+kubebuilder:printcolumn:name="Availability",type=string,JSONPath=`.spec.availability`
//+kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.status`
//+kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.status.replicas`
//+kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`

// Shop is the Schema for the shops API
type Shop struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ShopSpec   `json:"spec,omitempty"`
	Status ShopStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ShopList contains a list of Shop
type ShopList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Shop `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Shop{}, &ShopList{})
}
