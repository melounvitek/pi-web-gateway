# AGENTS.md

## Git workflow

- By default, make changes directly on `master`.
- This repository has a GitHub remote.

## Local server

The dev server usually runs as Puma on `100.103.198.74:4567`, logging to `/tmp/pi-web-gateway.log`.

Restart pattern:

```bash
ps -ef | rg 'puma .*pi-web-gateway|rackup|config.ru' | rg -v rg
kill <pid>
nohup mise exec -- bundle exec rackup -o 100.103.198.74 -p 4567 > /tmp/pi-web-gateway.log 2>&1 &
```

Do not restart it unless explicitly asked; for code changes, tell the user a restart is needed.
