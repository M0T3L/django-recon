#!/bin/bash
set -e

echo "=================================================="
echo "🚀 Starting Django Recon Server Automatic Setup"
echo "=================================================="

# 1. Update system & install OS packages
echo "📦 Updating system packages & dependencies..."
apt-get update && apt-get install -y git build-essential curl wget unzip

# 2. Install Go 1.22.4
if ! command -v go &> /dev/null; then
    echo "🐹 Installing Go 1.22.4..."
    wget -q https://go.dev/dl/go1.22.4.linux-amd64.tar.gz
    rm -rf /usr/local/go && tar -C /usr/local -xzf go1.22.4.linux-amd64.tar.gz
    rm -f go1.22.4.linux-amd64.tar.gz
fi

export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
if ! grep -q "/usr/local/go/bin" ~/.bashrc; then
    echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> ~/.bashrc
fi

# 3. Install Recon CLI tools
echo "🛠️ Installing Recon CLI tools (subfinder, dnsx, naabu, httpx, katana, nuclei)..."
/usr/local/go/bin/go install -v github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest
/usr/local/go/bin/go install -v github.com/projectdiscovery/dnsx/cmd/dnsx@latest
/usr/local/go/bin/go install -v github.com/projectdiscovery/naabu/v2/cmd/naabu@latest
/usr/local/go/bin/go install -v github.com/projectdiscovery/httpx/cmd/httpx@latest
/usr/local/go/bin/go install -v github.com/projectdiscovery/katana/cmd/katana@latest
/usr/local/go/bin/go install -v github.com/projectdiscovery/nuclei/v3/cmd/nuclei@latest

# 4. Set up repository in /opt/django-recon
echo "📥 Setting up project directory at /opt/django-recon..."
mkdir -p /opt
cd /opt
if [ -d "django-recon" ]; then
    cd django-recon
    git fetch origin
    git reset --hard origin/main
else
    git clone https://github.com/M0T3L/django-uc.git django-recon
    cd django-recon
fi

mkdir -p logs web/static/screenshots

echo "🏗️ Building Go server binary..."
CGO_ENABLED=1 /usr/local/go/bin/go build -ldflags="-s -w" -o recon-server cmd/server/main.go

# 5. Create .env configuration file if missing
if [ ! -f .env ]; then
    echo "⚙️ Creating .env configuration..."
    cat <<EOF > .env
APP_ENV=production
PORT=8080
DB_PATH=/opt/django-recon/recon.db
BASIC_AUTH_USER=admin
BASIC_AUTH_PASS=3c95e49fe757
EOF
fi

# 6. Create & enable systemd service
echo "📌 Configuring systemd service (/etc/systemd/system/django-recon.service)..."
cat <<EOF > /etc/systemd/system/django-recon.service
[Unit]
Description=Django Recon Automation & ASM Platform
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/django-recon
ExecStart=/opt/django-recon/recon-server
Restart=always
RestartSec=5
Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/go/bin:/root/go/bin
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now django-recon

echo "=================================================="
echo "🎉 INSTALLATION COMPLETED SUCCESSFULLY!"
echo "=================================================="
echo "🌐 Web Dashboard  : http://188.132.234.82:8080"
echo "👤 Basic Auth User: admin"
echo "🔑 Basic Auth Pass: 3c95e49fe757"
echo "--------------------------------------------------"
echo "📊 Service status command: systemctl status django-recon"
echo "📜 Live logs command     : journalctl -u django-recon -f"
echo "=================================================="
