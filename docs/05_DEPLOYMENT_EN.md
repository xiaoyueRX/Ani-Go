# Deploy to a NAS or home server

[简体中文](05_DEPLOYMENT.md) · [Quick start](../README_EN.md#quick-start)

## Containers

For v0.7.0 on an ARM64 NAS, use the release archive `ani-go-v0.7.0-linux-arm64.tar.gz`. The ARM64 variant of `ghcr.io/xiaoyuerx/ani-go:0.7.0` contains an x86-64 executable and fails with `exec format error` on hosts with native ARM64 execution only. [v0.7.0 release assets](https://github.com/xiaoyueRX/Ani-Go/releases/tag/v0.7.0)

1. In the repository directory, copy `.env.example` to `.env` and enter the downloader address and credentials. Use a container service name or a host address reachable from Ani-Go.
2. Replace `/path/to/your/data` in Compose with an existing NAS share. An empty directory works for a fresh install when the container account can create files and subdirectories there. The default container user is `1000:1000`. If the share already grants write access to another account, set its numeric `PUID` and `PGID` in `.env` before starting. Use `docker run --user UID:GID` for standalone containers.
3. Run `docker compose up -d ani-go`. Visit `http://localhost:20001`, or use the NAS address from another device. On first startup you can set `ANIGO_ADMIN_PASSWORD`. Otherwise the generated administrator password is printed in `docker compose logs ani-go`. [Compose file](../docker-compose.yml) · [Example environment](../.env.example)

| Container path | Purpose |
| --- | --- |
| `/data/db/ani-go.db` | Persistent SQLite database containing accounts, subscriptions, settings and plugin configuration. |
| `/data/backups` | Compose backup directory, configurable with `BACKUP_PATH`. |
| `/data/uploads/avatars`, `/data/ani-go.log` | Uploaded avatars and the application log. The Compose service runs with `/` as its working directory. |
| `/data/media` | Compose places series, films and specials in separate directories beneath this path. |
| `/data/downloads` | One possible download directory. The completed path reported by the downloader must be readable by Ani-Go. |

For a new container created in a NAS interface or with `docker run`, mount `/data`, set the working directory to `/`, and use the database, backup and three media paths from the Compose configuration to reproduce this layout. With only the `/data` mount and its default working directory, the official v0.7.0 image writes the database to `/app/ani-go.db` inside the container. [Compose configuration](../docker-compose.yml)

Leave a subscription’s **Download directory** blank to use the default save settings of Transmission, aria2 or qBittorrent. To override it, enter a literal path understood by the downloader, such as `/data/downloads/series`. Clearing the field restores the default for subsequent downloads. The organizer places completed media in the configured library.

When the download directory is blank, the scheduler still checks the Ani-Go host’s `TV_BASE_PATH` volume against `DISK_MIN_FREE_GB` before submission. An explicit download directory is checked using the existing path or its nearest existing parent on the Ani-Go host. Monitor free space on a remote downloader’s default volume on that machine.

On Synology, fnOS or Unraid, choose one shared parent directory and mount it as `/data` in both containers. This lets Ani-Go resolve the paths reported by the downloader. Hard links require both directories to be on the same filesystem and mount. A copy across filesystems consumes additional disk space. Enable `USE_HARDLINK` to retain the download source for seeding. [Organizer implementation](../internal/organizer/organizer.go)

Before organizing, Ani-Go asks qBittorrent, Transmission or Aria2 which files were selected and have finished downloading. It checks each local video's size against the client. Unselected episodes and unrelated videos in the same folder stay in place. If the client cannot confirm the files, the organizer waits for a later poll.

Move mode stages the complete file on the media library's filesystem, commits the episode record, then removes the download source. A failed database write leaves the source available for retry. After a restart, Ani-Go resumes pending cleanup. It keeps the source if the download file has changed or the library file is missing. To continue seeding, enable hard links. Ani-Go keeps the download source both when linking on the same filesystem and when copying across filesystems. On local Linux ext4, Btrfs, XFS, F2FS or tmpfs volumes that support directory locking, the next organization in the same directory reclaims abandoned Ani-Go staging files. Windows, network shares and other volumes use isolated staging directories. A power loss during a copy on these volumes can leave a hidden staging file. [Organization commit and recovery](../internal/scheduler/organizer_commit.go)

The default series layout uses `Title (Year)/Season 1` or `Title/Season 1` when the year is unknown. Jellyfin can use the parentheses to identify the title and year separately. After an upgrade, if the only existing directory for the same title and year uses the older `TitleYear` layout, later episodes and seasons continue there. You can set a custom TV template in Settings.

## Backups and restore

Point `BACKUP_PATH` at a writable directory on a persistent volume. Manual and scheduled backups share `BACKUP_KEEP_COUNT`, which defaults to the latest seven archives. The default schedule is midnight. `BACKUP_CRON` accepts five fields, such as `0 3 * * *`, or six fields including seconds. Saving either setting applies it to the running backup scheduler. Deployment environment variables take precedence.

Before upgrading, create and download a backup in Settings. Include episodes if you need their records. The administrator JSON contains settings, subscriptions and optional episodes and may include plugin credentials. Keep it private. The persistent SQLite database retains accounts and the local session signing key. Ani-Go creates the first administrator when the user table is empty. Startup reports an error if it cannot read the database or the stored signing key is damaged. Restore database access or recover the signing key from a working copy of this instance’s SQLite database before restarting. New archives carry a SHA-256 integrity digest checked before restoration. Older archives without a digest remain readable. When restoring across databases, subscriptions with the same title and season retain their source, group and filter settings and their respective episodes. Plugins reload after a successful restore. Restart to apply other startup configuration. [Backup API](API.md)

## Upgrade and verify

Check that backups are on a persistent volume. Keep the existing host directory mounted at `/data`. Before recreating an older container, copy its `/app/data/uploads` to `uploads` on the host share if it contains avatars. Retrieve any backups or historical logs still under `/app/data` as well. Run the commands below, then check the health response's `version` and the container logs.

```bash
docker compose pull ani-go
docker compose up -d ani-go
curl -fsS http://localhost:20001/api/health
```

### Trying a release candidate and rolling back

When moving a manually created container to a persistent volume, locate its actual database first. If the file is in the container's writable layer, stop and retain that container. Copy the database and any adjacent `-wal` and `-shm` files together to a private temporary directory. Use SQLite's `.backup` on this copy to create a snapshot and check that `PRAGMA quick_check` returns `ok`. Place the snapshot at `db/ani-go.db` on a new, empty volume, give the container user read and write access, and verify the original login and subscriptions before retiring the old container.

Record the running image digest or keep the working executable, Compose file and environment settings. On the host, use SQLite’s `.backup` command on the active `DB_PATH` to create a separate snapshot. Run `PRAGMA quick_check` on the snapshot and verify it returns `ok`. The SQLite backup interface includes committed WAL transactions. If Compose uses `latest`, pin `image` to the candidate tag or digest before the trial.

To roll back, stop Ani-Go and preserve the upgraded database and any `-wal` and `-shm` sidecars together in a separate directory. Restore the pre-upgrade snapshot to the original `DB_PATH` with its original ownership and permissions. Start the previously recorded image digest or executable. Check the health endpoint’s version, sign in with the existing account, and verify subscription seasons and organized episode paths. The database snapshot restores application records. Check media files and download-client jobs separately.

## Standalone installation

On Linux or macOS, the installer fetches the GitHub release archive, verifies it against the release’s `checksums.txt` and installs the executable for the current user. Set `ANIGO_VERSION` on the shell running the installer to select a tag. For example, append `| ANIGO_VERSION=v0.7.0 sh` in place of `| sh` below.

```sh
curl -fsSL https://raw.githubusercontent.com/xiaoyueRX/Ani-Go/main/install.sh | sh
cd "$HOME/.anigo"
"$HOME/.local/bin/anigo"
```

The working directory holds the active `.env` and relative database paths. Stop a running standalone process before upgrading, rerun the installer, then restart it from that same directory. Uninstall with `sh install.sh --uninstall` from a checkout to remove the managed executable and keep your configuration and data. For manual deployment, including Windows, download the platform archive, verify its checksum and launch `./anigo` or `anigo.exe` in a writable working directory. For MCP behind a reverse proxy, check its SSE paths and token configuration. [Releases](https://github.com/xiaoyueRX/Ani-Go/releases) · [MCP connection guide](03_MCP_SPEC_EN.md)
