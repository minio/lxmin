# Systemd service for Lxmin

Systemd script for `lxmin`.

## Installation

- Systemd script is configured to run the binary from /usr/local/bin/
- Download the binary from https://github.com/minio/lxmin/releases

## Create default configuration

```sh
$ cat <<EOF >> /etc/default/lxmin

## MinIO endpoint configuration
LXMIN_ENDPOINT=http://localhost:9000
LXMIN_BUCKET="backups"
LXMIN_ACCESS_KEY="minioadmin"
LXMIN_SECRET_KEY="minioadmin"

## LXMIN address
LXMIN_ADDRESS=":8000"

## LXMIN server certificate and client trust certs.
LXMIN_TLS_CERT="/var/snap/lxd/common/lxd/server.crt"
LXMIN_TLS_KEY="/var/snap/lxd/common/lxd/server.key"
LXMIN_TLS_CAPATH="/var/snap/lxd/capath"
EOF
```

## Systemctl

Download lxmin.service in /etc/systemd/system/

## Enable startup on boot

```
systemctl enable lxmin.service
```

## Note
Replace User=nobody and Group=nobody in lxmin.service file with your local setup user.
