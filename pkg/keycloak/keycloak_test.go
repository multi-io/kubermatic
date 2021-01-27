// +build keycloaktest

package keycloak

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func requireEnv(key string, t *testing.T) string {
	value, ok := os.LookupEnv(key)
	if !ok {
		t.Fatalf("Required environment variable not set: %s", key)
	}
	return value
}

func TestAuth(t *testing.T) {
	kc := NewClient(requireEnv("KEYCLOAK_EXTERNAL_URL", t), requireEnv("KEYCLOAK_EXTERNAL_ADMIN_USER", t), requireEnv("KEYCLOAK_EXTERNAL_ADMIN_PASSWORD", t))
	token, err := kc.getAdminToken()
	if err != nil {
		t.Fatalf("Error: %v\n", err)
	}
	fmt.Printf("token: %v\n", token)
}

func TestGetClientData(t *testing.T) {
	kc := NewClient(requireEnv("KEYCLOAK_EXTERNAL_URL", t), requireEnv("KEYCLOAK_EXTERNAL_ADMIN_USER", t), requireEnv("KEYCLOAK_EXTERNAL_ADMIN_PASSWORD", t))

	cd, err := kc.GetClientData("testkunde", "metakube-cluster")
	if err != nil {
		t.Fatalf("gcd Error: %v\n", err)
	}

	fmt.Printf("issuerUrl: %v, client ID: %s, client secret: %v\n", cd.IssuerURL, cd.ClientID, cd.ClientSecret)
}

func TestGetClientDataNotFound(t *testing.T) {
	kc := NewClient(requireEnv("KEYCLOAK_EXTERNAL_URL", t), requireEnv("KEYCLOAK_EXTERNAL_ADMIN_USER", t), requireEnv("KEYCLOAK_EXTERNAL_ADMIN_PASSWORD", t))
	doTestGetClientDataNotFound(kc, t)
}

func doTestGetClientDataNotFound(kc Facade, t *testing.T) {
	_, err := kc.GetClientData("testkunde", "metakube-clust")

	if _, ok := err.(*ClientNotFoundError); ok {
		fmt.Printf("OK, received ClientNotFoundError %v\n", err)
	} else {
		t.Fatalf("Unexpected error: %v", err)
	}

	_, err = kc.GetClientData("testkun", "metakube-cluster")

	if _, ok := err.(*RealmNotFoundError); ok {
		fmt.Printf("OK, received RealmNotFoundError %v\n", err)
	} else {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestClientGroup(t *testing.T) {
	kc1 := NewClient(requireEnv("KEYCLOAK_EXTERNAL_URL", t), requireEnv("KEYCLOAK_EXTERNAL_ADMIN_USER", t), requireEnv("KEYCLOAK_EXTERNAL_ADMIN_PASSWORD", t))
	kc2 := NewClient(requireEnv("KEYCLOAK_INTERNAL_URL", t), requireEnv("KEYCLOAK_INTERNAL_ADMIN_USER", t), requireEnv("KEYCLOAK_INTERNAL_ADMIN_PASSWORD", t))

	kg := NewGroup()
	kg.RegisterKeycloak(NewCache(kc1, 1*time.Second))
	kg.RegisterKeycloak(NewCache(kc2, 1*time.Second))

	cd, err := kg.GetClientData("testkunde", "metakube-cluster")
	if err != nil {
		t.Fatalf("gcd Error: %v\n", err)
	}

	fmt.Printf("issuerUrl: %v, client ID: %s, client secret: %v\n", cd.IssuerURL, cd.ClientID, cd.ClientSecret)

	cd, err = kg.GetClientData("syseleven", "metakube-cluster")
	if err != nil {
		t.Fatalf("gcd Error: %v\n", err)
	}

	fmt.Printf("issuerUrl: %v, client ID: %s, client secret: %v\n", cd.IssuerURL, cd.ClientID, cd.ClientSecret)
}

func TestGroupGetClientDataNotFound(t *testing.T) {
	kc1 := NewClient(requireEnv("KEYCLOAK_EXTERNAL_URL", t), requireEnv("KEYCLOAK_EXTERNAL_ADMIN_USER", t), requireEnv("KEYCLOAK_EXTERNAL_ADMIN_PASSWORD", t))
	kc2 := NewClient(requireEnv("KEYCLOAK_INTERNAL_URL", t), requireEnv("KEYCLOAK_INTERNAL_ADMIN_USER", t), requireEnv("KEYCLOAK_INTERNAL_ADMIN_PASSWORD", t))

	kg := NewGroup()
	kg.RegisterKeycloak(NewCache(kc1, 1*time.Second))
	kg.RegisterKeycloak(NewCache(kc2, 1*time.Second))

	doTestGetClientDataNotFound(kg, t)
}
