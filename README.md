# R Mailer Go SDK

Small Go client for R Mailer, RCorp's internal transactional email service.

The SDK intentionally stays narrow for `v0.2.0`: it sends template emails, supports optional idempotency keys, and reads message records. It does not acquire OAuth tokens, retry requests, manage queues, or build templates.

## Installation

```sh
go get github.com/Rcorporation/r-mailer-go-sdk
```

## Basic Usage

```go
package main

import (
	"context"
	"log"

	rmailer "github.com/Rcorporation/r-mailer-go-sdk"
)

func main() {
	ctx := context.Background()
	mailer := rmailer.New("https://mailer.rcorp.cc")

	msg, err := mailer.SendTemplate(ctx, "access-token", rmailer.SendTemplateRequest{
		From:           "no-reply@rcorp.cc",
		To:             []string{"user@example.com"},
		Template:       "identity.verify_email",
		Locale:         "en",
		IdempotencyKey: "kratos-courier-message-id",
		Data: map[string]any{
			"name":             "Sewon",
			"code":             "123456",
			"verification_url": "https://identity.rcorp.cc/verify/...",
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Println(msg.ID, msg.Status)
}
```

## R Identity Integration

R Mailer expects JWT access tokens issued by R Identity with:

- scope: `r-mailer:mail:send`
- audience: `r-mailer`

Example using `github.com/Rcorporation/r-identity-go-sdk`:

```go
package main

import (
	"context"
	"log"

	ridentity "github.com/Rcorporation/r-identity-go-sdk"
	rmailer "github.com/Rcorporation/r-mailer-go-sdk"
)

func main() {
	ctx := context.Background()

	identity, err := ridentity.New(ridentity.Config{
		Issuer:       "https://oauth.rcorp.cc",
		ClientID:     "r-identity",
		ClientSecret: "client-secret",
	})
	if err != nil {
		log.Fatal(err)
	}

	token, err := identity.ClientCredentials(ctx, ridentity.ClientCredentialsOptions{
		Audience: []string{"r-mailer"},
		Scope:    []string{"r-mailer:mail:send"},
	})
	if err != nil {
		log.Fatal(err)
	}

	mailer := rmailer.New("https://mailer.rcorp.cc")
	_, err = mailer.SendTemplate(ctx, token.AccessToken, rmailer.SendTemplateRequest{
		From:           "no-reply@rcorp.cc",
		To:             []string{"user@example.com"},
		Template:       "identity.password_reset",
		Locale:         "en",
		IdempotencyKey: "kratos-courier-message-id",
		Data: map[string]any{
			"name":      "Sewon",
			"code":      "123456",
			"reset_url": "https://identity.rcorp.cc/reset/...",
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

## API

```go
client := rmailer.New("https://mailer.rcorp.cc")
```

```go
msg, err := client.SendTemplate(ctx, accessToken, rmailer.SendTemplateRequest{
	From:           "no-reply@rcorp.cc",
	To:             []string{"user@example.com"},
	Template:       "identity.verify_email",
	Locale:         "en",
	Data:           map[string]any{"name": "Sewon"},
	IdempotencyKey: "kratos-courier-message-id",
})
```

`IdempotencyKey` is optional, but strongly recommended for at-least-once dispatchers. R Mailer deduplicates by authenticated client and idempotency key, returning the existing message instead of sending a duplicate.

```go
msg, err := client.GetMessage(ctx, accessToken, "message-id")
```

## Errors

Non-2xx API responses return `*rmailer.HTTPError`:

```go
var apiErr *rmailer.HTTPError
if errors.As(err, &apiErr) {
	log.Println(apiErr.StatusCode, apiErr.APIError.Error, apiErr.APIError.Message)
}
```

## Future Extension Points

- Optional client configuration struct
- Request/response metadata hooks
- Lightweight retry policy for idempotent reads
- Additional typed helpers for common R Identity templates

Retries, queueing, template builders, provider integrations, and OAuth token acquisition are intentionally outside `v0.2.0`.
