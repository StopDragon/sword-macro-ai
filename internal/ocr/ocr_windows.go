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

type windowsEngine struct {
	tempDir    string
	scriptPath string
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

	// PowerShell 스크립트 파일 생성
	scriptContent := `param([string]$imagePath)

Add-Type -AssemblyName System.Runtime.WindowsRuntime

$null = [Windows.Media.Ocr.OcrEngine,Windows.Foundation,ContentType=WindowsRuntime]
$null = [Windows.Storage.StorageFile,Windows.Storage,ContentType=WindowsRuntime]
$null = [Windows.Graphics.Imaging.BitmapDecoder,Windows.Graphics.Imaging,ContentType=WindowsRuntime]

function Await($WinRtTask, $ResultType) {
    $asTaskGeneric = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object { $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1' })[0]
    $asTask = $asTaskGeneric.MakeGenericMethod($ResultType)
    $netTask = $asTask.Invoke($null, @($WinRtTask))
    $netTask.Wait(-1) | Out-Null
    return $netTask.Result
}

try {
    $ocrEngine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage([Windows.Globalization.Language]::new("ko"))
    if ($ocrEngine -eq $null) {
        $ocrEngine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
    }
    if ($ocrEngine -eq $null) {
        exit 1
    }

    $storageFile = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($imagePath)) ([Windows.Storage.StorageFile])
    $stream = Await ($storageFile.OpenAsync([Windows.Storage.FileAccessMode]::Read)) ([Windows.Storage.Streams.IRandomAccessStream])
    $decoder = Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
    $bitmap = Await ($decoder.GetSoftwareBitmapAsync()) ([Windows.Graphics.Imaging.SoftwareBitmap])
    $ocrResult = Await ($ocrEngine.RecognizeAsync($bitmap)) ([Windows.Media.Ocr.OcrResult])

    foreach ($line in $ocrResult.Lines) {
        Write-Output $line.Text
    }

    $stream.Dispose()
} catch {
    exit 1
}
`
	e.scriptPath = filepath.Join(e.tempDir, "ocr.ps1")
	if err := os.WriteFile(e.scriptPath, []byte(scriptContent), 0644); err != nil {
		return err
	}

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

	// PowerShell 스크립트 실행
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", e.scriptPath, "-imagePath", tempFile)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
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

// CheckKoreanOCRInstalled Windows에서 한국어 OCR 언어팩 설치 확인
func CheckKoreanOCRInstalled() error {
	script := `$lang = [Windows.Globalization.Language]::new("ko"); $ocr = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($lang); if ($ocr -eq $null) { exit 1 }`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if err := cmd.Run(); err != nil {
		return errors.New("한국어 OCR 언어팩이 설치되지 않았습니다")
	}
	return nil
}
