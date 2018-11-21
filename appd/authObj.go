package controller

import (
	"fmt"
)

type AppDRestAuth struct {
	Token     string
	SessionID string
}

func NewRestAuth(t string, s string) AppDRestAuth {
	return AppDRestAuth{Token: t, SessionID: s}
}

func (ra *AppDRestAuth) getAuthCookie() string {
	return fmt.Sprintf("X-CSRF-TOKEN=%s; JSESSIONID=%s", ra.Token, ra.SessionID)
}
