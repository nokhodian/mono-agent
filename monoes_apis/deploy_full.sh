#!/bin/bash
set -e

IP="18.199.91.233"
KEY="deploy_key.pem"
DOMAIN="apiv1.monoes.me"
ZONE_ID="Z062787384KQWDBDUKUH"
API_KEY="AIzaSyAoGR8ZqHgLHHVtOmNtBezVr-VtoBUCPw0"

echo "=== 1. Setting up Domain Record ==="
CHANGE_BATCH=$(cat <<EOF
{
  "Comment": "Update record for $DOMAIN",
  "Changes": [
    {
      "Action": "UPSERT",
      "ResourceRecordSet": {
        "Name": "$DOMAIN",
        "Type": "A",
        "TTL": 300,
        "ResourceRecords": [
          {
            "Value": "$IP"
          }
        ]
      }
    }
  ]
}
EOF
)
aws route53 change-resource-record-sets --hosted-zone-id $ZONE_ID --change-batch "$CHANGE_BATCH"

echo "=== 2. Creating Nginx Config ==="
cat > nginx.conf <<EOF
server {
    listen 80;
    server_name $DOMAIN;
    client_max_body_size 50M;

    location / {
        proxy_pass http://127.0.0.1:8000;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF

echo "=== 3. Preparing Server ==="
ssh -o StrictHostKeyChecking=no -i $KEY ubuntu@$IP "
    export DEBIAN_FRONTEND=noninteractive
    sudo apt-get update
    sudo apt-get install -y python3-pip python3-venv nginx acl
    mkdir -p monoes_apis
"

echo "=== 4. Syncing Files ==="
rsync -avz --exclude 'venv' --exclude '.git' --exclude '__pycache__' --exclude '*.pyc' --exclude 'deploy_key.pem' -e "ssh -o StrictHostKeyChecking=no -i $KEY" . ubuntu@$IP:~/monoes_apis/

echo "=== 5. Configuring Server ==="
ssh -o StrictHostKeyChecking=no -i $KEY ubuntu@$IP <<EOF
    set -e
    cd monoes_apis

    # Setup Virtual Environment
    if [ ! -d "venv" ]; then
        python3 -m venv venv
    fi
    source venv/bin/activate
    pip install -r requirements.txt

    # Setup .env
    echo "GEMINI_API_KEY=$API_KEY" > .env
    echo "DEBUG=true" >> .env

    # Setup Systemd Service
    sudo bash -c 'cat > /etc/systemd/system/monoes.service <<SERVICE
[Unit]
Description=Monoes API
After=network.target

[Service]
User=ubuntu
WorkingDirectory=/home/ubuntu/monoes_apis
ExecStart=/home/ubuntu/monoes_apis/venv/bin/uvicorn main:app --host 0.0.0.0 --port 8000 --log-level debug
Restart=always

[Install]
WantedBy=multi-user.target
SERVICE'

    sudo systemctl daemon-reload
    sudo systemctl enable monoes
    sudo systemctl restart monoes

    # Setup Nginx
    sudo mv nginx.conf /etc/nginx/sites-available/monoes
    sudo ln -sf /etc/nginx/sites-available/monoes /etc/nginx/sites-enabled/
    sudo rm -f /etc/nginx/sites-enabled/default
    sudo nginx -t
    sudo systemctl restart nginx

    echo "=== Deployment Complete ==="
EOF
