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

// ClearClipboard 클립보드 비우기 (복사 실패 시 이전 내용 반환 방지)
func ClearClipboard() {
	clearClipboard()
}

// ReadChatText 채팅 영역에서 텍스트 읽기 (클립보드 복사 방식)
// chatX, chatY: 채팅 영역 클릭 좌표
// inputX, inputY: 입력창 좌표 (복귀용)
func ReadChatText(chatX, chatY, inputX, inputY int) string {
	// 1. 채팅 영역 클릭 (텍스트 선택 가능하도록)
	Click(chatX, chatY)
	time.Sleep(50 * time.Millisecond)

	// 2. 전체 선택 (Cmd+A / Ctrl+A)
	SelectAll()
	time.Sleep(50 * time.Millisecond)

	// 3. 복사 (Cmd+C / Ctrl+C)
	CopySelection()
	time.Sleep(100 * time.Millisecond)

	// 4. 클립보드에서 텍스트 가져오기
	text := GetClipboard()

	// 5. 입력창으로 복귀 (선택 해제)
	Click(inputX, inputY)
	time.Sleep(20 * time.Millisecond)

	return text
}

// SendCommand 게임 명령어 전송
func SendCommand(x, y int, command string) {
	// 1. 입력창 클릭
	Click(x, y)
	time.Sleep(30 * time.Millisecond)

	// 2. 입력창 청소 (Cmd+A → Delete)
	ClearInput()
	time.Sleep(60 * time.Millisecond)

	// 3. 텍스트 입력 (클립보드 + Cmd+V)
	TypeText(command)
	time.Sleep(150 * time.Millisecond)

	// 4. 엔터 2번 (줄바꿈 + 전송)
	PressEnter()
	time.Sleep(200 * time.Millisecond)
	PressEnter()
	time.Sleep(50 * time.Millisecond)
}

// SendCommandOnce 게임 명령어 전송 (엔터 1번만)
// 입력창 클리어 후 텍스트 입력, 엔터 1번 (줄바꿈만, 전송 안됨)
func SendCommandOnce(x, y int, command string) {
	// 1. 입력창 클릭
	Click(x, y)
	time.Sleep(30 * time.Millisecond)

	// 2. 입력창 청소 (Cmd+A → Delete)
	ClearInput()
	time.Sleep(60 * time.Millisecond)

	// 3. 텍스트 입력 (클립보드 + Cmd+V)
	TypeText(command)
	time.Sleep(150 * time.Millisecond)

	// 4. 엔터 1번만 (줄바꿈)
	PressEnter()
	time.Sleep(50 * time.Millisecond)
}

// AppendAndSend 기존 입력에 텍스트 추가 후 전송 (엔터 2번)
// 입력창을 클리어하지 않고 텍스트를 추가한 뒤 전송
// 주의: 클릭하면 커서가 맨 앞으로 가므로 클릭하지 않음
// 카카오톡: /프로(엔터1번) + @유저명(엔터2번) = 메시지 전송
func AppendAndSend(x, y int, text string) {
	// 클릭 없이 바로 텍스트 추가 (이전 단계에서 커서가 이미 끝에 있음)
	TypeText(text)
	time.Sleep(150 * time.Millisecond)

	// 엔터 2번 (전송)
	PressEnter()
	time.Sleep(200 * time.Millisecond)
	PressEnter()
	time.Sleep(50 * time.Millisecond)
}

// CheckFailsafe 비상 정지 체크 (화면 좌상단)
func CheckFailsafe() bool {
	x, y := GetMousePos()
	return x <= 5 && y <= 5
}
