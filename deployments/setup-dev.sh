#!/bin/bash
set -e

# Dev 服务器初始化脚本
# 用法: ssh dev 'bash -s' < setup-dev.sh

DOMAIN="cockpit.cuihairu.site"
DEPLOY_DIR="/opt/cockpit"
NGINX_CONF="/etc/nginx/sites-available/${DOMAIN}"

echo "=== Cockpit Dev 服务器初始化 ==="

# 1. 创建部署目录
echo "创建部署目录..."
sudo mkdir -p ${DEPLOY_DIR}/web
sudo mkdir -p /var/lib/cockpit
sudo mkdir -p /etc/cockpit

# 2. 创建 cockpit 用户（如果不存在）
if ! id cockpit &>/dev/null; then
    echo "创建 cockpit 用户..."
    sudo useradd --system --user-group --home-dir /var/lib/cockpit --shell /usr/sbin/nologin cockpit
fi
sudo chown -R cockpit:cockpit /var/lib/cockpit

# 3. 配置 nginx
echo "配置 nginx..."
sudo tee ${NGINX_CONF} > /dev/null << 'NGINX_EOF'
server {
    listen 80;
    server_name cockpit.cuihairu.site;

    location /.well-known/acme-challenge/ {
        root /var/www/html;
    }

    location / {
        return 301 https://$server_name$request_uri;
    }
}

server {
    listen 443 ssl http2;
    server_name cockpit.cuihairu.site;

    # 自签名证书（后续可用 ACME 替换）
    ssl_certificate /etc/nginx/ssl/${DOMAIN}/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/${DOMAIN}/privkey.pem;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 1d;
    ssl_session_tickets off;

    add_header Strict-Transport-Security "max-age=63072000" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;

    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    location / {
        proxy_pass http://127.0.0.1:9000;
        proxy_buffering off;
    }

    location /ws {
        proxy_pass http://127.0.0.1:9000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400;
    }

    location /api/agents/ {
        client_max_body_size 50m;
        proxy_pass http://127.0.0.1:9000;
        proxy_read_timeout 300;
        proxy_send_timeout 300;
    }

    location ~ ^/api/remote/(terminal|desktop|vnc) {
        proxy_pass http://127.0.0.1:9000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400;
    }

    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        proxy_pass http://127.0.0.1:9000;
        expires 7d;
        add_header Cache-Control "public, immutable";
    }

    location /health {
        proxy_pass http://127.0.0.1:9000;
        access_log off;
    }
}
NGINX_EOF

# 4. 生成自签名证书（临时）
echo "生成自签名证书..."
sudo mkdir -p /etc/nginx/ssl/${DOMAIN}
if [ ! -f /etc/nginx/ssl/${DOMAIN}/fullchain.pem ]; then
    sudo openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
        -keyout /etc/nginx/ssl/${DOMAIN}/privkey.pem \
        -out /etc/nginx/ssl/${DOMAIN}/fullchain.pem \
        -subj "/CN=${DOMAIN}" \
        -addext "subjectAltName=DNS:${DOMAIN}"
fi

# 5. 启用站点
echo "启用 nginx 站点..."
sudo ln -sf ${NGINX_CONF} /etc/nginx/sites-enabled/${DOMAIN}
sudo nginx -t
sudo systemctl reload nginx

# 6. 创建 systemd 服务
echo "创建 cockpit systemd 服务..."
sudo tee /etc/systemd/system/cockpit.service > /dev/null << 'EOF'
[Unit]
Description=Cockpit Server
Documentation=https://github.com/cuihairu/cockpit
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=cockpit
Group=cockpit
EnvironmentFile=-/etc/default/cockpit-server
ExecStart=/opt/cockpit/cockpit server -config /etc/cockpit/config.yaml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=cockpit
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/cockpit /opt/cockpit
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

# 7. 创建默认配置（如果不存在）
if [ ! -f /etc/cockpit/config.yaml ]; then
    echo "创建默认配置..."
    sudo tee /etc/cockpit/config.yaml > /dev/null << 'EOF'
server:
  host: 127.0.0.1
  port: 9000
  static_dir: /opt/cockpit/web

database:
  path: /var/lib/cockpit/cockpit.db

inventory:
  path: /var/lib/cockpit/inventory.yaml
  watch: true

jwt:
  secret: change-me-in-production
  expiration: 24h
EOF
fi

if [ ! -f /etc/default/cockpit-server ]; then
    echo "创建环境配置..."
    sudo tee /etc/default/cockpit-server > /dev/null << 'EOF'
ADMIN_USERNAME=admin
ADMIN_PASSWORD=change-this-password
PRODUCTION=true
ALLOWED_ORIGINS=https://cockpit.cuihairu.site
EOF
    sudo chmod 600 /etc/default/cockpit-server
fi

# 8. 重载 systemd
sudo systemctl daemon-reload

echo ""
echo "=== 初始化完成 ==="
echo ""
echo "下一步:"
echo "1. 配置 GitHub Actions self-hosted runner（见下方说明）"
echo "2. 修改 /etc/default/cockpit-server 设置管理员密码"
echo "3. 等待 CI/CD 部署或手动部署"
echo ""
echo "nginx 配置: ${NGINX_CONF}"
echo "部署目录: ${DEPLOY_DIR}"
echo "数据目录: /var/lib/cockpit"
echo "配置文件: /etc/cockpit/config.yaml"
echo "环境文件: /etc/default/cockpit-server"