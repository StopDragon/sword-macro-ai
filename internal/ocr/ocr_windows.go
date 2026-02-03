//go:build windows

package ocr

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Windows OCR은 PowerShell을 통해 Windows.Media.Ocr API 호출
// 네이티브 WinRT 바인딩보다 간단하고 안정적

type windowsEngine struct {
	tempDir string
}

func newEngine() Engine {
	return &windowsEngine{}
}

func (e *windowsEngine) Init() error {
	// 임시 디렉토리 생성
	tempDir, err := os.MkdirTemp("", "sword-ocr-")
	if err != nil {
		return err
	}
	e.tempDir = tempDir
	return nil
}

func (e *windowsEngine) Recognize(img *image.RGBA) (string, error) {
	if img == nil {
		return "", nil
	}

	// 이미지를 임시 PNG 파일로 저장
	tempFile := filepath.Join(e.tempDir, "capture.png")
	f, err := os.Create(tempFile)
	if err != nil {
		return "", err
	}

	if err := png.Encode(f, img); err != nil {
		f.Close()
		return "", err
	}
	f.Close()

	// PowerShell로 Windows OCR API 호출
	script := `
Add-Type -AssemblyName System.Runtime.WindowsRuntime

$asyncInfo = [Windows.Media.Ocr.OcrEngine,Windows.Foundation,ContentType=WindowsRuntime]
$null = [Windows.Storage.StorageFile,Windows.Storage,ContentType=WindowsRuntime]
$null = [Windows.Graphics.Imaging.BitmapDecoder,Windows.Graphics.Imaging,ContentType=WindowsRuntime]

function Await($WinRtTask, $ResultType) {
    $asTask = [System.WindowsRuntimeSystemExtensions].GetMethods() |
        Where-Object { $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1' } |
        Select-Object -First 1
    $netTask = $asTask.MakeGenericMethod($ResultType).Invoke($null, @($WinRtTask))
    $netTask.Wait(-1) | Out-Null
    return $netTask.Result
}

function AwaitAction($WinRtTask) {
    $asTask = [System.WindowsRuntimeSystemExtensions].GetMethods() |
        Where-Object { $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncAction' } |
        Select-Object -First 1
    $netTask = $asTask.Invoke($null, @($WinRtTask))
    $netTask.Wait(-1) | Out-Null
}

$imagePath = '` + strings.ReplaceAll(tempFile, "\\", "\\\\") + `'

# 한국어 OCR 엔진 (없으면 영어 사용)
$ocrEngine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage([Windows.Globalization.Language]::new("ko"))
if ($ocrEngine -eq $null) {
    $ocrEngine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
}

$storageFile = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($imagePath)) ([Windows.Storage.StorageFile])
$stream = Await ($storageFile.OpenAsync([Windows.Storage.FileAccessMode]::Read)) ([Windows.Storage.Streams.IRandomAccessStream])
$decoder = Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
$bitmap = Await ($decoder.GetSoftwareBitmapAsync()) ([Windows.Graphics.Imaging.SoftwareBitmap])

$ocrResult = Await ($ocrEngine.RecognizeAsync($bitmap)) ([Windows.Media.Ocr.OcrResult])

$lines = @()
foreach ($line in $ocrResult.Lines) {
    $lines += $line.Text
}

$stream.Dispose()
Write-Output ($lines -join "`n")
`

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// PowerShell 에러 시 빈 문자열 반환 (정상 동작)
		return "", nil
	}

	result := strings.TrimSpace(stdout.String())
	return result, nil
}

func (e *windowsEngine) Close() {
	if e.tempDir != "" {
		os.RemoveAll(e.tempDir)
	}
}

// Windows에서 한국어 OCR 언어팩 설치 확인
func CheckKoreanOCRInstalled() error {
	script := `
$lang = [Windows.Globalization.Language]::new("ko")
$ocrEngine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($lang)
if ($ocrEngine -eq $null) {
    Write-Output "NOT_INSTALLED"
} else {
    Write-Output "INSTALLED"
}
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	output, err := cmd.Output()
	if err != nil {
		return errors.New("OCR 상태 확인 실패")
	}

	if strings.TrimSpace(string(output)) == "NOT_INSTALLED" {
		return errors.New("한국어 OCR 언어팩이 설치되지 않았습니다. 설정 > 시간 및 언어 > 언어에서 한국어를 추가하세요")
	}

	return nil
}
