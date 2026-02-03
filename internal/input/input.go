package input

import "time"

// Click 마우스 클릭
// 플랫폼별 구현은 input_darwin.go, input_windows.go에서 제공
func Click(x, y int) {
	click(x, y)
}

// Move 마우스 이동
func Move(x, y int) {
	move(x, y)
}

// GetMousePos 마우스 현재 위치
func GetMousePos() (int, int) {
	return getMousePos()
}

// TypeText 텍스트 입력 (클립보드 + 붙여넣기)
func TypeText(text string) {
	typeText(text)
}

// PressEnter 엔터 키 입력
func PressEnter() {
	pressEnter()
}

// SendCommand 게임 명령어 전송 (클릭 + 텍스트 + 엔터)
func SendCommand(x, y int, command string) {
	Click(x, y)
	time.Sleep(150 * time.Millisecond)
	TypeText(command)
	time.Sleep(200 * time.Millisecond) // 붙여넣기 완료 대기
	PressEnter()
	time.Sleep(100 * time.Millisecond) // 엔터 처리 대기
}

// CheckFailsafe 비상 정지 체크 (화면 좌상단)
func CheckFailsafe() bool {
	x, y := GetMousePos()
	return x <= 5 && y <= 5
}
