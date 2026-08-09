# immich-go distribution

Single-binary Immich server (Go port). Packages for:
- linux/arm64, linux/amd64, darwin/arm64, darwin/amd64, windows/amd64

Each package contains the binary, a README.txt, and a launcher
(start.sh / start.bat). Extract and run.

## Verify
sha256sum -c checksums.txt

## Run (Linux/macOS)
tar xzf immich-go-1.0.0-go-linux-arm64.tar.gz
cd immich-go-1.0.0-go-linux-arm64
./start.sh            # or: ./immich-go
# open http://localhost:8081  (admin@immich.app / password)

## systemd (Linux)
See immich-go.service — copy it to /etc/systemd/system/ and run:
  sudo systemctl enable --now immich-go
