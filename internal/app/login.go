package app

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"golang.org/x/term"
)

func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// interactiveFlow builds an auth flow that prompts for phone, code and (only if
// Telegram asks) a 2FA password on stdin.
func interactiveFlow() auth.Flow {
	in := bufio.NewReader(os.Stdin)
	code := auth.CodeAuthenticatorFunc(func(ctx context.Context, _ *tg.AuthSentCode) (string, error) {
		return prompt(in, "Login code: "), nil
	})
	return auth.NewFlow(&userAuth{code: code, in: in}, auth.SendCodeOptions{})
}

type userAuth struct {
	code auth.CodeAuthenticator
	in   *bufio.Reader
}

func (u *userAuth) Phone(context.Context) (string, error) {
	p := prompt(u.in, "Phone number (international, e.g. +15551234567): ")
	if p == "" {
		return "", fmt.Errorf("phone number is required")
	}
	return p, nil
}

func (u *userAuth) Code(ctx context.Context, s *tg.AuthSentCode) (string, error) {
	return u.code.Code(ctx, s)
}

func (u *userAuth) Password(context.Context) (string, error) {
	fmt.Fprint(os.Stderr, "2FA password: ")
	if stdinIsTerminal() {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return strings.TrimSpace(string(b)), err
	}
	return prompt(u.in, ""), nil
}

func (u *userAuth) AcceptTermsOfService(context.Context, tg.HelpTermsOfService) error {
	return nil
}

func (u *userAuth) SignUp(context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("sign up not supported; use an existing account")
}

func prompt(in *bufio.Reader, label string) string {
	if label != "" {
		fmt.Fprint(os.Stderr, label)
	}
	line, _ := in.ReadString('\n')
	return strings.TrimSpace(line)
}
