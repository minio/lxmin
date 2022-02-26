# lxmin

Backup and restore LXC instances with MinIO

## Install

### Binary Releases

| OS       | ARCH    | Binary                                                                                           |
|:--------:|:-------:|:------------------------------------------------------------------------------------------------:|
| Linux    | amd64   | [linux-amd64](https://github.com/minio/lxmin/releases/latest/download/lxmin-linux-amd64)         |
| Linux    | arm64   | [linux-arm64](https://github.com/minio/lxmin/releases/latest/download/lxmin-linux-arm64)         |
| Linux    | ppc64le | [linux-ppc64le](https://github.com/minio/lxmin/releases/latest/download/lxmin-linux-ppc64le)     |
| Linux    | s390x   | [linux-s390x](https://github.com/minio/lxmin/releases/latest/download/lxmin-linux-s390x)         |
| Apple M1 | arm64   | [darwin-arm64](https://github.com/minio/lxmin/releases/latest/download/lxmin-darwin-arm64)       |
| Apple    | amd64   | [darwin-amd64](https://github.com/minio/lxmin/releases/latest/download/lxmin-darwin-amd64)       |
| Windows  | amd64   | [windows-amd64](https://github.com/minio/lxmin/releases/latest/download/lxmin-windows-amd64.exe) |

### Usage

```sh
NAME:
  lxmin - backup and restore LXC instances with MinIO

USAGE:
  lxmin [FLAGS] COMMAND [COMMAND FLAGS | -h] [ARGUMENTS...]

COMMANDS:
  backup      backup an instance image to MinIO
  restore     restore an instance image from MinIO
  info        pretty print tags on an instance image on MinIO
  list, ls    list all backups from MinIO
  delete, rm  deletes a specific backup by 'name' for an instance from MinIO

GLOBAL FLAGS:
  --endpoint value    endpoint for MinIO server [$LXMIN_ENDPOINT]
  --bucket value      bucket to save/restore backup(s) [$LXMIN_BUCKET]
  --access-key value  access key credential [$LXMIN_ACCESS_KEY]
  --secret-key value  secret key credential [$LXMIN_SECRET_KEY]
  --address value     enable TLS REST API service [$LXMIN_ADDRESS]
  --cert value        TLS server certificate [$LXMIN_TLS_CERT]
  --key value         TLS server private key [$LXMIN_TLS_KEY]
  --capath value      TLS trust certs for incoming clients [$LXMIN_TLS_CAPATH]
  --help, -h          show help

ENVIRONMENT VARIABLES:
  LXMIN_ENDPOINT      endpoint for MinIO server
  LXMIN_BUCKET        bucket to save/restore backup(s)
  LXMIN_ACCESS_KEY    access key credential
  LXMIN_SECRET_KEY    secret key credential
  LXMIN_ADDRESS       run as HTTPs REST API service
```

## REST API

`lxmin` exposes an mTLS authentication based REST API. HTTPs is mandatory for this service so you would need relevant server and client public certs. We recommend that you re-use your LXD server certificates.

```sh
export LXMIN_ENDPOINT=http://147.75.71.77:9000
export LXMIN_BUCKET="backups"
export LXMIN_ACCESS_KEY="minioadmin"
export LXMIN_SECRET_KEY="minioadmin"
export LXMIN_ADDRESS=":8000"
export LXMIN_TLS_CERT="/var/snap/lxd/common/lxd/server.crt"
export LXMIN_TLS_KEY="/var/snap/lxd/common/lxd/server.key"
export LXMIN_TLS_CAPATH="${HOME}/.lxc/"

lxmin
2022/02/26 08:31:51 Server listening on :8000
```

The spirit of this this API is to be close to LXD REST API documentation, authentication shall be achieved using the similar mTLS based authentication as per LXD REST API documentation <https://linuxcontainers.org/lxd/docs/master/api/>

| Method | API                                    | Desc                                                                                                                     |
|:-------|:---------------------------------------|:-------------------------------------------------------------------------------------------------------------------------|
| GET    | /1.0/instances/{name}/backups          | Get the backups (Returns a list of instance backups on MinIO, along with some addtional metadata)                        |
| GET    | /1.0/instances/{name}/backups/{backup} | Get backup specific metadata and information                                                                             |
| POST   | /1.0/instances/{name}/backups          | Create a backup to MinIO (Optionally you can add x-amz-tagging: "key=value" format to add additional tags on the backup) |
| DELETE | /1.0/instances/{name}/backups/{backup} | Delete a backup from MinIO                                                                                               |
| POST   | /1.0/instances/{name}/backups/{backup} | Restore a backup from MinIO                                                                                              |

Response type for this API will be always `application/json`

### GET /1.0/instances/{name}/backups

Response example:

```json
{
  "metadata": [
 {
   "name": "backup_2022-02-17-08-3732.tar.gz",
   "created": "2022-02-17T08:38:47.609Z",
   "size": 913921606,
   "optimized": true,
   "compressed": false
 },
 {
   "name": "backup_2022-02-17-09-0524.tar.gz",
   "created": "2022-02-17T09:06:39.324Z",
   "size": 913898354,
   "optimized": true,
   "compressed": false
 },
 {
   "name": "backup_2022-02-17-09-3329.tar.gz",
   "created": "2022-02-17T09:34:44.868Z",
   "size": 913879736,
   "optimized": true,
   "compressed": false
 },
 {
   "name": "backup_2022-02-26-07-3921.tar.gz",
   "created": "2022-02-26T07:41:07.868Z",
   "size": 1303072030,
   "optimized": true,
   "compressed": true
 }
  ],
  "status": "Success",
  "status_code": 200,
  "type": "sync"
}
```

### GET /1.0/instances/{name}/backups/{backup}

Response example:

```json
{
  "metadata": {
 "name": "backup_2022-02-26-08-2027.tar.gz",
 "created": "2022-02-26T08:22:12Z",
 "size": 1302974262,
 "optimized": true,
 "compressed": true,
 "tags": {
   "os": "Ubuntu",
   "version": "20.04"
 }
  },
  "status": "Success",
  "status_code": 200,
  "type": "sync"
}
```

### POST /1.0/instances/{name}/backups

Response example:

```json
{
  "metadata": {
 "name": "backup_2022-02-26-07-5218.tar.gz",
 "optimized": true,
 "compressed": true
  },
  "status": "Operation created",
  "status_code": 100,
  "type": "async"
}
```

### DELETE /1.0/instances/{name}/backups/{backup}

Response example:

```json
{
  "status": "Success",
  "status_code": 200,
  "type": "sync"
}
```

### POST /1.0/instances/{name}/backups/{backup}

Response example:

```json
{
  "status": "Operation created",
  "status_code": 100,
  "type": "async"
}
```

## CLI (command line)

`lxmin` can be run as a manual tool to manage your `lxc` backups.

### Create a backup

```sh
export LXMIN_ENDPOINT=http://147.75.71.77:9000
export LXMIN_BUCKET="backups"
export LXMIN_ACCESS_KEY="minioadmin"
export LXMIN_SECRET_KEY="minioadmin"

lxmin backup u2 --tags "OS=Ubuntu&Version=20.04&Build=10"
Preparing backup for (u2) instance: success
Uploading backup_2022-02-17-09-3329.tar.gz ┃▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓┃ 100.12 MiB/s
```

### Create an optimized backup

Optimize flag enables faster restore time. It is only supported for ZFS, BTRFS, RBD based storage pools.

```sh
lxmin backup u2 --optimize
Preparing backup for (u2) instance: success
Uploading backup_2022-02-17-09-3329.tar.gz ┃▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓┃ 100.12 MiB/s
```

### List all backups

```sh
lxmin list
┌──────────┐┌──────────────────────────────────┐┌─────────────────────────┐┌─────────┐┌───────────┐
│ Instance ││ Name                             ││ Created                 ││ Size    ││ Optimized │
│          ││                                  ││                         ││         ││           │
│ u2       ││ backup_2022-02-17-08-3732.tar.gz ││ 2022-02-17 08:38:47 UTC ││ 872 MiB ││ ✔         │
│ u2       ││ backup_2022-02-17-09-0524.tar.gz ││ 2022-02-17 09:06:39 UTC ││ 872 MiB ││ ✔         │
│ u2       ││ backup_2022-02-17-09-3329.tar.gz ││ 2022-02-17 09:34:44 UTC ││ 872 MiB ││ ✔         │
└──────────┘└──────────────────────────────────┘└─────────────────────────┘└─────────┘└───────────┘
```

### Restore a backup

```sh
lxmin restore u2 backup_2022-02-17-09-3329.tar.gz
Downloading backup_2022-02-17-09-3329.tar.gz ┃▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓┃ 209.10 MiB/s
Launching instance (u2) from backup: success
```

### Display backup info

```sh
Name      : backup_2022-02-17-09-3329.tar.gz
Date      : 2022-02-17 09:34:44 UTC
Size      : 872 MiB
Tags      :
  Version   : 20.04
  Build     : 10
  OS        : Ubuntu
Metadata  :
  Optimized : ✔
  Compressed: ✔
```

### Delete a backup

Delete a backup by name `backup_2022-02-16-04-1040.tar.gz`

```sh
lxmin delete u2 backup_2022-02-16-04-1040.tar.gz
Backup backup_2022-02-16-04-1040.tar.gz deleted successfully
```

Delete all backups (dangerous operation)

```sh
lxmin delete u2 --all --force
All backups for u2 deleted successfully
```
