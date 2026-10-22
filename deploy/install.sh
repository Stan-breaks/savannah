#!/usr/bin/env bash
#
# Sets the savannah API up on a server. Run it as root:
#
#   sudo ./deploy/install.sh
#
# It is safe to run it again.
set -euo pipefail

REPO_URL="${REPO_URL:-https://github.com/Stan-breaks/savannah.git}"
BRANCH="${BRANCH:-main}"
APP_DIR=/var/www/savannah
CACHE_DIR=/var/cache/savannah

[ "$(id -u)" -eq 0 ] || { echo "run this as root" >&2; exit 1; }
for cmd in git go systemctl ssh-keygen; do
    command -v "$cmd" >/dev/null || { echo "$cmd is not installed" >&2; exit 1; }
done

# The user the service and the deploy script run as.
if ! id -u web >/dev/null 2>&1; then
    useradd --system --home-dir "$APP_DIR" --shell /bin/bash web
    # '*' means no password login, the runner uses a key.
    usermod -p '*' web
fi

install -d -m 0755 -o web -g web "$APP_DIR" "$CACHE_DIR"

# The checkout.
if [ -d "$APP_DIR/.git" ]; then
    sudo -u web git -C "$APP_DIR" pull --ff-only origin "$BRANCH"
else
    sudo -u web git clone --branch "$BRANCH" "$REPO_URL" "$APP_DIR"
fi

# The build.
sudo -u web env \
    HOME="$APP_DIR" GOPATH="$CACHE_DIR/go" GOCACHE="$CACHE_DIR/go-build" CGO_ENABLED=1 \
    go -C "$APP_DIR" build -o "$APP_DIR/savannah" .

# A ssh session does not load the shell profile, so go has to be somewhere
# on the default path.
if [ ! -x /usr/local/bin/go ]; then
    ln -sf "$(command -v go)" /usr/local/bin/go
fi

# The unit.
install -m 0644 "$APP_DIR/deploy/savannah.service" /etc/systemd/system/savannah.service
systemctl daemon-reload

# The deploy script may restart the service, nothing else.
SYSTEMCTL="$(command -v systemctl)"
echo "web ALL=(root) NOPASSWD: $SYSTEMCTL restart savannah.service" > /etc/sudoers.d/savannah
chmod 0440 /etc/sudoers.d/savannah
if command -v visudo >/dev/null; then
    visudo -c -f /etc/sudoers.d/savannah
fi

# The key the GitHub Actions runner uses to get in.
#
# By default a keypair is generated here and the private half is printed
# at the end. If you would rather keep the private key off the server,
# point DEPLOY_PUBKEY at a public key file and it is used instead.
install -d -m 0700 -o web -g web "$APP_DIR/.ssh"
if [ -n "${DEPLOY_PUBKEY:-}" ]; then
    cp "$DEPLOY_PUBKEY" "$APP_DIR/.ssh/deploy_key.pub"
    chown web:web "$APP_DIR/.ssh/deploy_key.pub"
    echo "using the public key from $DEPLOY_PUBKEY"
elif [ ! -f "$APP_DIR/.ssh/deploy_key" ]; then
    ssh-keygen -t ed25519 -N '' -C 'savannah-deploy' -f "$APP_DIR/.ssh/deploy_key"
    chown web:web "$APP_DIR/.ssh/deploy_key" "$APP_DIR/.ssh/deploy_key.pub"
fi
touch "$APP_DIR/.ssh/authorized_keys"
grep -qF "$(cat "$APP_DIR/.ssh/deploy_key.pub")" "$APP_DIR/.ssh/authorized_keys" \
    || cat "$APP_DIR/.ssh/deploy_key.pub" >> "$APP_DIR/.ssh/authorized_keys"
chown web:web "$APP_DIR/.ssh/authorized_keys"
chmod 0600 "$APP_DIR/.ssh/authorized_keys"

systemctl enable savannah.service
systemctl restart savannah.service
systemctl --no-pager --lines=0 status savannah.service || true

echo
echo "Done. Add these to the GitHub repository settings"
echo "(Settings, Secrets and variables, Actions):"
echo
echo "  DEPLOY_HOST      the public address of this server"
echo "  DEPLOY_SSH_PORT  the ssh port, usually 22"
echo "  DEPLOY_USER      web"
echo "  DEPLOY_SSH_KEY   the private key that matches the public key below"
echo
if [ -f "$APP_DIR/.ssh/deploy_key" ]; then
    echo "DEPLOY_SSH_KEY:"
    cat "$APP_DIR/.ssh/deploy_key"
else
    echo "The public key came from outside, so there is no private key here."
    echo "Paste the private half of $DEPLOY_PUBKEY into DEPLOY_SSH_KEY."
fi
echo
echo "The public key on the server:"
cat "$APP_DIR/.ssh/deploy_key.pub"
echo
