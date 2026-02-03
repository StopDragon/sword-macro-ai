# Makefile for sword-macro-ai

APP_NAME := SwordMacro
VERSION := 2.0.0
BUILD_DIR := build
CMD_DIR := cmd/sword-macro

# Go 설정
GO := go
GOFLAGS := -ldflags="-s -w"

# 플랫폼별 설정
DARWIN_AMD64 := GOOS=darwin GOARCH=amd64
DARWIN_ARM64 := GOOS=darwin GOARCH=arm64
WINDOWS_AMD64 := GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc

.PHONY: all clean build-mac build-mac-arm64 build-windows deps

all: clean deps build-mac

# 의존성 설치
deps:
	@echo "📦 의존성 설치 중..."
	$(GO) mod tidy
	$(GO) mod download

# macOS 빌드 (현재 아키텍처)
build-mac:
	@echo "🔨 macOS 빌드 중..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(APP_NAME) ./$(CMD_DIR)
	@echo "✅ 빌드 완료: $(BUILD_DIR)/$(APP_NAME)"
	@ls -lh $(BUILD_DIR)/$(APP_NAME)

# macOS ARM64 (Apple Silicon)
build-mac-arm64:
	@echo "🔨 macOS ARM64 빌드 중..."
	@mkdir -p $(BUILD_DIR)
	$(DARWIN_ARM64) $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-arm64 ./$(CMD_DIR)
	@echo "✅ 빌드 완료: $(BUILD_DIR)/$(APP_NAME)-darwin-arm64"

# macOS AMD64 (Intel)
build-mac-amd64:
	@echo "🔨 macOS AMD64 빌드 중..."
	@mkdir -p $(BUILD_DIR)
	$(DARWIN_AMD64) $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(APP_NAME)-darwin-amd64 ./$(CMD_DIR)
	@echo "✅ 빌드 완료: $(BUILD_DIR)/$(APP_NAME)-darwin-amd64"

# macOS Universal Binary
build-mac-universal: build-mac-arm64 build-mac-amd64
	@echo "🔨 Universal Binary 생성 중..."
	lipo -create -output $(BUILD_DIR)/$(APP_NAME)-darwin-universal \
		$(BUILD_DIR)/$(APP_NAME)-darwin-arm64 \
		$(BUILD_DIR)/$(APP_NAME)-darwin-amd64
	@echo "✅ Universal Binary 완료: $(BUILD_DIR)/$(APP_NAME)-darwin-universal"

# Windows 빌드 (크로스 컴파일 - mingw 필요)
build-windows:
	@echo "🔨 Windows 빌드 중..."
	@echo "⚠️  Windows 빌드는 Windows에서 실행하거나 mingw-w64가 필요합니다."
	@mkdir -p $(BUILD_DIR)
	$(WINDOWS_AMD64) $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(APP_NAME).exe ./$(CMD_DIR)
	@echo "✅ 빌드 완료: $(BUILD_DIR)/$(APP_NAME).exe"

# 개발용 실행
run:
	$(GO) run ./$(CMD_DIR)

# 테스트
test:
	$(GO) test -v ./...

# 정리
clean:
	@echo "🗑️  정리 중..."
	rm -rf $(BUILD_DIR)
	@echo "✅ 정리 완료"

# 크기 확인
size:
	@echo "📊 빌드 크기:"
	@ls -lh $(BUILD_DIR)/* 2>/dev/null || echo "빌드 파일 없음"

# 도움말
help:
	@echo "사용법:"
	@echo "  make deps          - 의존성 설치"
	@echo "  make build-mac     - macOS 빌드 (현재 아키텍처)"
	@echo "  make build-mac-universal - macOS Universal Binary"
	@echo "  make build-windows - Windows 빌드 (크로스 컴파일)"
	@echo "  make run           - 개발 모드 실행"
	@echo "  make clean         - 빌드 정리"
	@echo "  make size          - 빌드 크기 확인"
