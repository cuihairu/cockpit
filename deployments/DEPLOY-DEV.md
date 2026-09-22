# Dev 环境部署指南

## 1. 在 dev 服务器上运行初始化脚本

```bash
ssh dev 'bash -s' < deployments/setup-dev.sh
```

## 2. 配置 GitHub Actions Self-hosted Runner

### 2.1 获取 Runner Token

1. 访问 https://github.com/cuihairu/cockpit/settings/actions/runners
2. 点击 "New self-hosted runner"
3. 选择 Linux x64
4. 复制 token（格式类似 `AXXXXXXXXXXXXXXXXXXXXXXX`）

### 2.2 在 dev 服务器上安装 Runner

```bash
ssh dev

# 创建目录
mkdir -p /opt/actions-runner && cd /opt/actions-runner

# 下载 runner
curl -o actions-runner-linux-x64-2.320.0.tar.gz -L https://github.com/actions/runner/releases/download/v2.320.0/actions-runner-linux-x64-2.320.0.tar.gz

# 解压
tar xzf ./actions-runner-linux-x64-2.320.0.tar.gz

# 配置（替换 YOUR_TOKEN）
./config.sh --url https://github.com/cuihairu/cockpit --token YOUR_TOKEN --labels dev --name dev-runner --work _work

# 安装为服务
sudo ./svc.sh install
sudo ./svc.sh start
sudo ./svc.sh status
```

### 2.3 验证 Runner

在 GitHub Actions 页面应该能看到状态为 "Idle" 的 runner，标签包含 `dev`。

## 3. 配置 GitHub Secrets

在 https://github.com/cuihairu/cockpit/settings/secrets/actions 添加：

| Secret | 说明 |
|--------|------|
| `ADMIN_PASSWORD` | 管理员密码 |
| `JWT_SECRET` | JWT 密钥（随机字符串） |

## 4. 配置 SSL 证书（可选）

### 方式一：Let's Encrypt

```bash
ssh dev

# 安装 certbot
sudo apt install certbot python3-certbot-nginx

# 获取证书
sudo certbot --nginx -d cockpit.cuihairu.site

# 自动续期
sudo certbot renew --dry-run
```

### 方式二：使用 Cockpit ACME 功能

在 Web UI 中配置 ACME 证书签发（需要 DNS 验证）。

## 5. 部署流程

推送到 `main` 分支会自动触发部署：

```bash
git add .
git commit -m "your changes"
git push origin main
```

部署日志：https://github.com/cuihairu/cockpit/actions/workflows/deploy-dev.yml

## 6. 访问

- **URL**: https://cockpit.cuihairu.site
- **Health**: https://cockpit.cuihairu.site/health

## 7. 常用命令

```bash
# 查看服务状态
ssh dev "sudo systemctl status cockpit"

# 查看日志
ssh dev "sudo journalctl -u cockpit -f"

# 重启服务
ssh dev "sudo systemctl restart cockpit"

# 查看 nginx 日志
ssh dev "sudo tail -f /var/log/nginx/error.log"
```

## 8. 回滚

```bash
ssh dev

# 查看备份
ls -la /opt/cockpit/cockpit.bak.*

# 恢复备份
sudo systemctl stop cockpit
sudo cp /opt/cockpit/cockpit.bak.YYYYMMDDHHMMSS /opt/cockpit/cockpit
sudo systemctl start cockpit
```