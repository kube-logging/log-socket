package internal

import (
	"context"

	authv1 "k8s.io/api/authentication/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type TokenReviewAuthenticator struct {
	Client client.Client `json:"client" yaml:"client"` //this is a client
}

func (t TokenReviewAuthenticator) Authenticate(token string) (res authv1.UserInfo, err error) {
	tr := authv1.TokenReview{Spec: authv1.TokenReviewSpec{Token: token}}
	if err = t.Client.Create(context.Background(), &tr); err != nil {
		return res, err
	}
	if !tr.Status.Authenticated {
		return res, unauthenticatedError{}
	}

	return tr.Status.User, nil
}

type unauthenticatedError struct{}

func (unauthenticatedError) Error() string {
	return "unauthenticated"
}

func (unauthenticatedError) IsUnauthenticatedError() bool {
	return true
}
