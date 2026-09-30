# Smoke test against a real account

Runs the provider from this checkout against a real KAS account in two
phases. Phase 1 reads every object type; phase 2 creates one disposable object
of each kind under a test label and destroys it again. Objects that exist
before the test are never changed: the configuration has no
`allinkl_domain_settings` and no `allinkl_tls_certificate`, because those
touch existing hosts.

Objects created in phase 2, all named after `label` (default `tfsmoke`): a
TXT record, a subdomain, a mailbox with responder, a forward to that mailbox,
an FTP login, a database, an inactive cronjob, a dynamic DNS login. A contract
limit such as `max_ftpuser_reached` fails that one resource and leaves the
rest; that is a finding, not damage.

```sh
cd examples/smoke
export KAS_LOGIN=w0123456 KAS_PASSWORD=...   # KAS_OTP for a 2FA account
export KAS_AUTH_TYPE=plain                    # if KAS answers kas_auth_type_disabled to the sha1 default
./run.sh example.com                          # phase 1: plan and apply data sources only
./run.sh example.com write                    # phase 2: plan, ask, apply, ask, destroy
```

`run.sh` builds the provider from this checkout and loads it through
`dev_overrides`, so no registry and no release is involved. Terraform state
lands in `.run/` and is git-ignored.

KAS applies some changes asynchronously and answers `in_progress` until
done; a retry after a minute settles it. Flood protection makes the write
phase take several minutes.
