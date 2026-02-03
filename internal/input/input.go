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

// ClearInput 입력창 내용 청소 (전체선택 + 삭제)
func ClearInput() {
	clearInput()
}

// SelectAll 전체 선택 (Cmd+A / Ctrl+A)
func SelectAll() {
	selectAll()
}

// CopySelection 선택 영역 복사 (Cmd+C / Ctrl+C)
func CopySelection() {
	copySelection()
}

// GetClipboard 클립보드 텍스트 가져오기
func GetClipboard() string {
	return getClipboard()
}

// ReadChatText 채팅 영역에서 텍스트 읽기 (클립보드 복사 방식)
// chatX, chatY: 채팅 영역 클릭 좌표
// inputX, inputY: 입력창 좌표 (복귀용)
func ReadChatText(chatX, chatY, inputX, inputY int) string {
	// 1. 채팅 영역 클릭 (텍스트 선택 가능하도록)
	Click(chatX, chatY)
	time.Sleep(100 * time.Millisecond)

	// 2. 전체 선택 (Cmd+A / Ctrl+A)
	SelectAll()
	time.Sleep(100 * time.Millisecond)

	// 3. 복사 (Cmd+C / Ctrl+C)
	CopySelection()
	time.Sleep(150 * time.Millisecond)

	// 4. 클립보드에서 텍스트 가져오기
	text := GetClipboard()

	// 5. 입력창으로 복귀 (선택 해제)
	Click(inputX, inputY)
	time.Sleep(50 * time.Millisecond)

	return text
}

// SendCommand 게임 명령어 전송
func SendCommand(x, y int, command string) {
	// 1. 입력창 클릭
	Click(x, y)
	time.Sleep(50 * time.Millisecond)

	// 2. 입력창 청소 (Cmd+A → Delete)
	ClearInput()
	time.Sleep(50 * time.Millisecond)

	// 3. 텍스트 입력 (클립보드 + Cmd+V)
	TypeText(command)
	time.Sleep(300 * time.Millisecond) // 붙여넣기 후 0.3초

	// 4. 엔터 2번 (0.3초 간격)
	PressEnter()
	time.Sleep(300 * time.Millisecond)
	PressEnter()
	time.Sleep(100 * time.Millisecond)
}

// CheckFailsafe 비상 정지 체크 (화면 좌상단)
func CheckFailsafe() bool {
	x, y := GetMousePos()
	return x <= 5 && y <= 5
}
