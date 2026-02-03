# 배포 가이드

## 서버 배포 (텔레메트리 API)

### 서버 정보
- 위치: `/opt/telemetry`
- 서비스: `telemetry.service`
- 포트: `8000` (systemd에서 `Environment=PORT=8000` 설정)

### 배포 명령어

```bash
cd /opt/telemetry
rm -rf temp
git clone https://github.com/StopDragon/sword-macro-ai.git temp
cd temp && go build -ldflags="-s -w" -o ../sword-api ./cmd/sword-api && cd ..
rm -rf temp
systemctl restart telemetry
```

### 서비스 관리

```bash
# 상태 확인
systemctl status telemetry

# 로그 확인
journalctl -u telemetry -f

# 재시작
systemctl restart telemetry

# 중지
systemctl stop telemetry
```

### systemd 설정 (/etc/systemd/system/telemetry.service)

```ini
[Service]
Type=simple
User=root
WorkingDirectory=/opt/telemetry
ExecStart=/opt/telemetry/sword-api
Environment=PORT=8000
Restart=always
RestartSec=5
```

---

## 클라이언트 빌드

### macOS (ARM64)
```bash
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o builds/sword-macro-darwin-arm64 ./cmd/sword-macro
```

### macOS (AMD64)
```bash
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o builds/sword-macro-darwin-amd64 ./cmd/sword-macro
```

### Windows (AMD64)
```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o builds/sword-macro-windows-amd64.exe ./cmd/sword-macro
```

### 전체 빌드 (로컬)
```bash
# macOS ARM64 (네이티브)
CGO_ENABLED=1 go build -ldflags="-s -w" -o builds/sword-macro-darwin-arm64 ./cmd/sword-macro

# macOS AMD64
CGO_ENABLED=1 GOARCH=amd64 go build -ldflags="-s -w" -o builds/sword-macro-darwin-amd64 ./cmd/sword-macro

# Windows (CGO 비활성화)
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o builds/sword-macro-windows-amd64.exe ./cmd/sword-macro
```

---

*마지막 업데이트: 2026-02-04 (v2.5.1)*
