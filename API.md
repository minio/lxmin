# External REST API

The spirit of this this API is to be close to LXD REST API documentation, authentication shall be achieved using the similar mTLS based authentication as per LXD REST API documentation https://linuxcontainers.org/lxd/docs/master/api/

| Method | API                                    | Desc                                                                                                                     |
|:-------|:---------------------------------------|:-------------------------------------------------------------------------------------------------------------------------|
| GET    | /1.0/instances/{name}/backups          | Get the backups (Returns a list of instance backups on MinIO, along with some addtional metadata)                        |
| GET    | /1.0/instances/{name}/backups/{backup} | Get backup specific metadata and information                                                                             |
| POST   | /1.0/instances/{name}/backups          | Create a backup to MinIO (Optionally you can add x-amz-tagging: "key=value" format to add additional tags on the backup) |
| DELETE | /1.0/instances/{name}/backups/{backup} | Delete a backup from MinIO                                                                                               |
| POST   | /1.0/instances/{name}/backups/{backup} | Restore a backup from MinIO                                                                                              |

Response type for this API will be always `application/json`

## GET /1.0/instances/{name}/backups

Response example:
```
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

## GET /1.0/instances/{name}/backups/{backup}

Response example:
```
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

## POST /1.0/instances/{name}/backups

Response example:
```
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

## DELETE /1.0/instances/{name}/backups/{backup}

Response example:
```
{
  "status": "Success",
  "status_code": 200,
  "type": "sync"
}
```

## POST /1.0/instances/{name}/backups/{backup}

Response example:
```
{
  "status": "Operation created",
  "status_code": 100,
  "type": "async"
}
```
