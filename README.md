# pi-backup

Backs up directories from a Raspberry Pi to S3, with restore support.

## Install

Download the `pi-backup` binary from [GitHub Releases](https://github.com/akostibas/pi-backup/releases) and place it in `/opt/pi-backup/`. The binary is cross-compiled for linux/arm64.

## Configuration

Create `/opt/pi-backup/config.yaml`:

```yaml
hostname: cherry
bucket: my-backup-bucket
region: us-east-1
directories:
  - /opt/homeassistant/config
  - /opt/pihole/etc-pihole
  - /opt/jellyfin/config
```

AWS credentials must be set as environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`). When running via systemd, use an `EnvironmentFile`.

## Systemd

Sample unit files are included in the repo:

- [`pi-backup.service`](pi-backup.service) -- oneshot service that runs the backup
- [`pi-backup.timer`](pi-backup.timer) -- daily timer with randomized delay

To install:

```bash
sudo cp pi-backup.service pi-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now pi-backup.timer
```

Create `/opt/pi-backup/env` with your AWS credentials:

```
AWS_ACCESS_KEY_ID=AKIA...
AWS_SECRET_ACCESS_KEY=...
```

## Usage

### Backup (default)

```bash
pi-backup                          # back up all directories
pi-backup --dry-run                # show what would happen
pi-backup --config /path/to/cfg    # use alternate config
```

Archives are uploaded to `s3://<bucket>/<hostname>/<dir-slug>/<timestamp>.tar.gz`.

### Restore

```bash
pi-backup restore list                          # list all backups
pi-backup restore list /opt/pihole/etc-pihole   # list backups for one dir
pi-backup restore /opt/pihole/etc-pihole        # restore latest
pi-backup restore /opt/pihole/etc-pihole --snapshot 2026-02-11T03-00-00Z
pi-backup restore /opt/pihole/etc-pihole --file etc-pihole/pihole-FTL.conf
pi-backup restore /opt/pihole/etc-pihole --dest /tmp/restore
```

## Skip-unchanged optimization

Each backup run creates a tar.gz archive and computes its SHA-256 hash. The hash is compared against the previous run's hash stored in `checksums.json` (same directory as the config file). If the hash matches, the upload is skipped.

Archives are deterministic -- filesystem access/change times are zeroed in tar headers so identical files always produce identical archives.

On the first run (or if `checksums.json` is missing), all directories are uploaded.

## AWS IAM permissions

The IAM user needs these S3 permissions on the backup bucket:

- `s3:PutObject` -- upload backups
- `s3:GetObject` -- download for restore
- `s3:ListBucket` -- list backups for restore
