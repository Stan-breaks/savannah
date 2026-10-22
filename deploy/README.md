# Deploying savannah

How the API gets onto a server, and how a merge to `main` gets it there.

There are three moving parts:

| File | Where it runs | What it does |
| --- | --- | --- |
| `deploy/savannah.service` | The server | The API, as a systemd service. |
| `deploy/deploy.sh` | The server | Pulls, rebuilds and restarts. |
| `deploy/seed.sh` | The server | Fills an empty database with the example clinic. |
| `deploy/install.sh` | The server | One-time setup. |
| `.github/workflows/deploy.yml` | GitHub | On a merge, ssh in and run `deploy.sh`. |

The flow on a merge:

```
merge into main
      │
      ▼
GitHub Actions ──ssh──▶ /var/www/savannah/deploy/deploy.sh
                                  │
                       git pull → go build → seed → systemctl restart
                                  │
                                  ▼
                         savannah.service
```

The runner has to authenticate with ssh, which is what keeps the server
from accepting a deployment from anyone.

## One-time setup on the server

The server needs Go, Git, SQLite and a C compiler, because `go-sqlite3` needs cgo:

```sh
sudo apt-get update
sudo apt-get install -y git golang build-essential sqlite3
```

Then run the install script:

```sh
git clone https://github.com/Stan-breaks/savannah.git
cd savannah
sudo ./deploy/install.sh
```

It creates the `web` user, clones the repository into `/var/www/savannah`,
builds the API, installs the systemd unit, writes the sudoers rule that
lets the deploy script restart the service, and generates a key for the
runner.

It is safe to run it again.

The layout it creates:

```
/var/www/savannah/        the checkout, and the built binary
/var/www/savannah/.ssh/   the key the runner uses
/var/cache/savannah/      the go build caches
```

The database is `sqlite.db` inside the checkout. It is created on the first
start and the schema is applied to it automatically. An empty database is
filled with the example clinic by `deploy/seed.sh`, which does nothing once
the database has doctors in it.

## GitHub secrets

Add these under **Settings → Secrets and variables → Actions**:

| Name | Value |
| --- | --- |
| `DEPLOY_HOST` | The public address of the server. |
| `DEPLOY_SSH_PORT` | The port the server's sshd listens on. Check it with `sudo ss -lntp \| grep sshd`, it is not always `22`. |
| `DEPLOY_USER` | `web`. |
| `DEPLOY_SSH_KEY` | The private key printed by `install.sh`. |

If you did not keep the output, the key is still on the server at
`/var/www/savannah/.ssh/deploy_key`.

## Trying it by hand

Over ssh, as `web`:

```sh
bash /var/www/savannah/deploy/deploy.sh
```

Or from anywhere, with `curl` against the running API:

```sh
curl "http://localhost:8089/doctors/1/availability?date=2026-10-24"
```

## What a deployment does

`deploy/deploy.sh` runs on the server:

```sh
cd /var/www/savannah
git pull --ff-only
go build -o savannah .
bash deploy/seed.sh
sudo systemctl restart savannah.service
```

`go build` swaps the new binary in with a rename, so the build works even
while the old one is still running, and the service is only restarted once
the new binary is in place. If the build fails, the old one keeps running.

`deploy/seed.sh` fills an empty database with the example clinic. It does
nothing once the database has doctors in it, so it is safe on every
deployment.

The checkout is treated as read-only apart from the build: only `git pull`
and `go build` write to it.

## Looking at what happened

```sh
sudo journalctl -u savannah -f
sudo systemctl status savannah
```

## If something goes wrong

**Permission denied on the ssh key.** The key in GitHub has to match the
public key in `/var/www/savannah/.ssh/authorized_keys` on the server.

**The build fails with a cgo error.** `gcc` is not installed on the server.

**`sudo: a password is required`.** The sudoers rule is missing. Check it
with `sudo visudo -c -f /etc/sudoers.d/savannah`.

**The service restarts and immediately exits.** It reads the schema from
`sqlc/schema.sql`, relative to its working directory, so the unit has to run
from inside the checkout. That is why `WorkingDirectory=/var/www/savannah`
is in the unit.

**`internal error: create slot 09:00: attempt to write a readonly database`.**
The service runs as `web`, and the database belongs to someone else. It
happens when the database is seeded by hand as root, for example with
`just seed` in the checkout. Hand it over and restart:

```sh
sudo chown web:web /var/www/savannah/sqlite.db
sudo systemctl restart savannah
```

To seed by hand, run it as the user the service runs as, so that the
database ends up with the right owner:

```sh
cd /var/www/savannah
sudo -u web sh -c 'sqlite3 sqlite.db < sqlc/schema.sql && sqlite3 sqlite.db < sqlc/seed.sql'
```
