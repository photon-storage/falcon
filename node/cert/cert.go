package cert

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/challenge/tlsalpn01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
)

type user struct {
	email        string
	registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *user) GetEmail() string {
	return u.email
}

func (u user) GetRegistration() *registration.Resource {
	return u.registration
}

func (u *user) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

func ObtainCert(
	email string,
	domain string,
	sk crypto.PrivateKey,
) ([]byte, []byte, error) {
	if sk == nil {
		var err error
		if sk, err = ecdsa.GenerateKey(
			elliptic.P256(),
			rand.Reader,
		); err != nil {
			return nil, nil, err
		}
	}

	u := user{
		email: email,
		key:   sk,
	}

	cli, err := lego.NewClient(lego.NewConfig(&u))
	if err != nil {
		return nil, nil, err
	}

	// It is required to have port 80 and 443 for answer challenges.
	if err := cli.Challenge.SetHTTP01Provider(
		http01.NewProviderServer("", "80"),
	); err != nil {
		return nil, nil, err
	}
	if err := cli.Challenge.SetTLSALPN01Provider(
		tlsalpn01.NewProviderServer("", "443"),
	); err != nil {
		return nil, nil, err
	}

	// New users will need to register
	reg, err := cli.Registration.Register(
		registration.RegisterOptions{TermsOfServiceAgreed: true},
	)
	if err != nil {
		return nil, nil, err
	}
	u.registration = reg

	// Each certificate comes back with the cert bytes, the bytes of
	// the client's private key, and a certificate URL.
	// SAVE THESE TO DISK.
	res, err := cli.Certificate.Obtain(certificate.ObtainRequest{
		Domains: []string{domain},
		Bundle:  true,
	})
	if err != nil {
		return nil, nil, err
	}

	return res.PrivateKey, res.Certificate, nil
}
