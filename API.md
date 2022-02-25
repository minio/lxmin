# External REST API

The spirit of this this API is to be close to LXD REST API documentation, authentication shall be achieved using the similar mTLS based authentication as per LXD REST API documentation https://linuxcontainers.org/lxd/docs/master/api/

| Method | API                                    | Desc                                                                                                                     |
|:-------|:---------------------------------------|:-------------------------------------------------------------------------------------------------------------------------|
| GET    | /1.0/instances/{name}/backups          | Get the backups (Returns a list of instance backups on MinIO, along with some addtional metadata)                        |
| GET    | /1.0/instances/{name}/backups/{backup} | Get backup specific metadata and information                                                                             |
| POST   | /1.0/instances/{name}/backups          | Create a backup to MinIO (Optionally you can add x-amz-tagging: "key=value" format to add additional tags on the backup) |
| DELETE | /1.0/instances/{name}/backups/{backup} | Delete a backup from MinIO                                                                                               |
| POST   | /1.0/instances/{name}/backups/{backup} | Restore a backup from MinIO                                                                                              |

## Response Type
Response type for this API will be always `application/json`
