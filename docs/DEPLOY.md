# Deploy

## Release flow

```
make check → git tag → git push --tags → CI builds → GitHub Release
```

### 1. Verify

```bash
make check
```

Runs `go vet` and all tests. Don't tag if this fails.

### 2. Tag

```bash
git tag v0.1.0
```

Use [semver](https://semver.org/):
- **Patch** (v0.1.1): bug fixes, no new features
- **Minor** (v0.2.0): new features, backward compatible
- **Major** (v1.0.0): breaking changes

### 3. Push

```bash
git push origin main --tags
```

This triggers two GitHub Actions workflows:

- **CI** (`ci.yml`): vet + test + build (runs on every push)
- **Release** (`release.yml`): cross-compile + publish (runs only on tags matching `v*`)

### 4. Binaries

The release workflow builds 5 binaries:

| Platform | Binary |
|----------|--------|
| Linux x64 | `gitcortex-linux-amd64` |
| Linux ARM64 | `gitcortex-linux-arm64` |
| macOS Intel | `gitcortex-darwin-amd64` |
| macOS Apple Silicon | `gitcortex-darwin-arm64` |
| Windows x64 | `gitcortex-windows-amd64.exe` |

All binaries have the version baked in via `-ldflags "-X main.version=v0.1.0"`.

### 5. GitHub Release

The release is created automatically with:
- All 5 binaries attached as downloadable assets
- Auto-generated release notes from commit messages since the last tag

Users download from: `https://github.com/lex0c/gitcortexv2/releases/latest`

## Install methods

### From release (no Go required)

```bash
# Linux
curl -L https://github.com/lex0c/gitcortexv2/releases/latest/download/gitcortex-linux-amd64 -o gitcortex
chmod +x gitcortex
sudo mv gitcortex /usr/local/bin/

# macOS (Apple Silicon)
curl -L https://github.com/lex0c/gitcortexv2/releases/latest/download/gitcortex-darwin-arm64 -o gitcortex
chmod +x gitcortex
sudo mv gitcortex /usr/local/bin/
```

### From source (Go required)

```bash
go install github.com/lex0c/gitcortexv2/cmd/gitcortex@latest
```

### From repo

```bash
git clone https://github.com/lex0c/gitcortexv2.git
cd gitcortexv2
make build
```

## Verify installation

```bash
gitcortex --version
```

Should show the tag version (e.g., `gitcortex version v0.1.0`).
