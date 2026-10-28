#!/usr/bin/env bash
# One-time setup for a fresh Ubuntu 24.04 Lightsail instance.
# Run as the default "ubuntu" user:
#   curl -fsSL https://raw.githubusercontent.com/vidarMaartensson/Lightpath/main/deploy/lightsail-setup.sh | bash
set -euo pipefail

# A 2 GB instance builds Go and the frontend comfortably; swap covers the
# build peak on smaller plans.
if ! swapon --show | grep -q /swapfile; then
  sudo fallocate -l 2G /swapfile
  sudo chmod 600 /swapfile
  sudo mkswap /swapfile
  sudo swapon /swapfile
  echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab >/dev/null
fi

# Docker Engine and the compose plugin from Docker's own repository.
if ! command -v docker >/dev/null; then
  curl -fsSL https://get.docker.com | sudo sh
  sudo usermod -aG docker "$USER"
fi

echo
echo "Docker is installed. Log out and back in so the docker group applies, then:"
echo "  git clone https://github.com/vidarMaartensson/Lightpath.git && cd Lightpath"
echo "  printf 'DOMAIN=<your domain>\\nPOSTGRES_PASSWORD=%s\\n' \"\$(openssl rand -hex 24)\" > .env"
echo "  make deploy"
echo "See \"Deploy to AWS\" in README.md for why the settings live in .env."
