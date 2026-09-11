# Authentication and authorization policy

## JWT configuration

JWT authentication requires these settings in `configs/yogourt.yaml`:

```yaml
security:
  secret_key: "${JWT_SECRET}" # at least 32 bytes
  token_issuer: "https://auth.example.com"
  token_audience: "example-api"
  token_expires: 60 # minutes; must be greater than zero
```

`services.CreateToken` emits `sub`, `exp`, `iss`, and `aud` claims and signs
with HS256. `services.ValidToken` accepts only HS256 and requires a valid
expiration plus the configured issuer and audience. Both functions reject an
incomplete JWT configuration. At application startup, an invalid policy is a
warning outside production and prevents startup in production.

Use a signing secret dedicated to this API and rotate or revoke tokens through
the application when accounts or credentials change.

## Authorization requests

Grant memoization remains limited to one request and is isolated by
authorization engine, subject, and scope. Composing multiple engines on the
same request therefore keeps each provider's grants independent.

Middleware refusals for anonymous requests return the same generic `401`
response and emit one `permission` decision event with reason
`unauthenticated` to each hook registered through `WithDecisionHook`.
