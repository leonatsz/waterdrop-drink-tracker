//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

const (
	wmCreate       = 0x0001
	wmDestroy      = 0x0002
	wmCommand      = 0x0111
	wmSetFont      = 0x0030
	wmUser         = 0x0400
	wsExTopMost    = 0x00000008
	wsOverlapped   = 0x00CF0000
	wsVisible      = 0x10000000
	wsChild        = 0x40000000
	wsTabStop      = 0x00010000
	wsBorder       = 0x00800000
	wsVScroll      = 0x00200000
	bsPushButton   = 0x00000000
	bsMultiline    = 0x00002000
	ssLeft         = 0x00000000
	ssCenter       = 0x00000001
	esNumber       = 0x00002000
	lbsNotify      = 0x00000001
	swHide         = 0
	swShow         = 5
	colorWindow    = 5
	idcArrow       = 32512
	pbmSetPos      = wmUser + 2
	pbmSetRange32  = wmUser + 6
	lbAddString    = 0x0180
	lbResetContent = 0x0184
	lbGetCurSel    = 0x0188
	lbErr          = ^uintptr(0)
	iccProgress    = 0x00000020
)

var (
	user32                   = syscall.NewLazyDLL("user32.dll")
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	gdi32                    = syscall.NewLazyDLL("gdi32.dll")
	comctl32                 = syscall.NewLazyDLL("comctl32.dll")
	procRegisterClassExW     = user32.NewProc("RegisterClassExW")
	procCreateWindowExW      = user32.NewProc("CreateWindowExW")
	procDefWindowProcW       = user32.NewProc("DefWindowProcW")
	procShowWindow           = user32.NewProc("ShowWindow")
	procUpdateWindow         = user32.NewProc("UpdateWindow")
	procGetMessageW          = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessageW     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procSendMessageW         = user32.NewProc("SendMessageW")
	procSetWindowTextW       = user32.NewProc("SetWindowTextW")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procEnableWindow         = user32.NewProc("EnableWindow")
	procLoadCursorW          = user32.NewProc("LoadCursorW")
	procLoadIconW            = user32.NewProc("LoadIconW")
	procGetModuleHandleW     = kernel32.NewProc("GetModuleHandleW")
	procCreateFontW          = gdi32.NewProc("CreateFontW")
	procInitCommonControlsEx = comctl32.NewProc("InitCommonControlsEx")

	hInstance uintptr
	fontUI    uintptr
	fontTitle uintptr
	mainWin   uintptr

	progressBar uintptr
	totalLabel  uintptr
	goalLabel   uintptr
	remaining   uintptr
	goalEdit    uintptr
	todayList   uintptr
	monthLabel  uintptr

	todayControls   []uintptr
	historyControls []uintptr
	calendarCells   [42]uintptr
	todayEntryIDs   []string
	viewingHistory  bool
	displayedMonth  time.Time
	data            AppData
)

type point struct{ X, Y int32 }
type message struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}
type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}
type initCommonControlsEx struct {
	DwSize uint32
	DwICC  uint32
}

type Entry struct {
	ID     string    `json:"id"`
	Amount int       `json:"amount"`
	At     time.Time `json:"at"`
}
type AppData struct {
	Goal    int     `json:"goal"`
	Entries []Entry `json:"entries"`
}

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func createFont(height int32, weight int32) uintptr {
	h, _, _ := procCreateFontW.Call(
		uintptr(height), 0, 0, 0, uintptr(weight), 0, 0, 0,
		1, 0, 0, 5, 0, uintptr(unsafe.Pointer(utf16Ptr("Segoe UI"))),
	)
	return h
}

func createControl(class, text string, style uint32, x, y, w, h int32, parent uintptr, id int) uintptr {
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr(class))),
		uintptr(unsafe.Pointer(utf16Ptr(text))),
		uintptr(style|wsChild|wsVisible),
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, uintptr(id), hInstance, 0,
	)
	procSendMessageW.Call(hwnd, wmSetFont, fontUI, 1)
	return hwnd
}

func dataFile() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "WaterDrop", "water.json")
}

func loadData() {
	data.Goal = 2000
	raw, err := os.ReadFile(dataFile())
	if err == nil {
		_ = json.Unmarshal(raw, &data)
	}
	if data.Goal < 500 {
		data.Goal = 2000
	}
}

func saveData() {
	path := dataFile()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	raw, _ := json.MarshalIndent(data, "", "  ")
	_ = os.WriteFile(path, raw, 0o644)
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func totalForDay(day time.Time) int {
	total := 0
	for _, entry := range data.Entries {
		if sameDay(entry.At.Local(), day) {
			total += entry.Amount
		}
	}
	return total
}

func setText(hwnd uintptr, text string) {
	procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(utf16Ptr(text))))
}

func getText(hwnd uintptr) string {
	n, _, _ := procGetWindowTextLengthW.Call(hwnd)
	buf := make([]uint16, n+1)
	procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func addWater(amount int) {
	data.Entries = append(data.Entries, Entry{
		ID:     strconv.FormatInt(time.Now().UnixNano(), 10),
		Amount: amount,
		At:     time.Now(),
	})
	saveData()
	refreshAll()
}

func refreshAll() {
	today := time.Now()
	total := totalForDay(today)
	percent := 0
	if data.Goal > 0 {
		percent = total * 100 / data.Goal
		if percent > 100 {
			percent = 100
		}
	}
	procSendMessageW.Call(progressBar, pbmSetRange32, 0, 100)
	procSendMessageW.Call(progressBar, pbmSetPos, uintptr(percent), 0)
	setText(totalLabel, fmt.Sprintf("今日 %d ml  ·  %d%%", total, percent))
	setText(goalLabel, fmt.Sprintf("每日目标：%d ml", data.Goal))
	remain := data.Goal - total
	if remain <= 0 {
		setText(remaining, "今天达标啦！")
	} else {
		setText(remaining, fmt.Sprintf("还差 %d ml", remain))
	}
	setText(goalEdit, strconv.Itoa(data.Goal))
	refreshTodayList()
	refreshCalendar()
}

func refreshTodayList() {
	procSendMessageW.Call(todayList, lbResetContent, 0, 0)
	todayEntryIDs = nil
	today := time.Now()
	var entries []Entry
	for _, entry := range data.Entries {
		if sameDay(entry.At.Local(), today) {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].At.After(entries[j].At) })
	for _, entry := range entries {
		label := fmt.Sprintf("%s    %d ml", entry.At.Local().Format("15:04"), entry.Amount)
		procSendMessageW.Call(todayList, lbAddString, 0, uintptr(unsafe.Pointer(utf16Ptr(label))))
		todayEntryIDs = append(todayEntryIDs, entry.ID)
	}
}

func refreshCalendar() {
	year, month, _ := displayedMonth.Date()
	setText(monthLabel, fmt.Sprintf("%d 年 %d 月", year, int(month)))
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	leading := (int(first.Weekday()) + 6) % 7
	days := time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local).Day()
	for i := 0; i < 42; i++ {
		dayNumber := i - leading + 1
		if dayNumber < 1 || dayNumber > days {
			setText(calendarCells[i], "")
			continue
		}
		day := time.Date(year, month, dayNumber, 0, 0, 0, 0, time.Local)
		total := totalForDay(day)
		if total > 0 {
			setText(calendarCells[i], fmt.Sprintf("%d\r\n%d ml", dayNumber, total))
		} else {
			setText(calendarCells[i], strconv.Itoa(dayNumber))
		}
	}
	now := time.Now()
	cy, cm, _ := now.Date()
	isCurrent := cy == year && cm == month
	procEnableWindow.Call(historyControls[2], boolToUintptr(!isCurrent))
}

func showHistory(show bool) {
	viewingHistory = show
	for _, hwnd := range todayControls {
		if show {
			procShowWindow.Call(hwnd, swHide)
		} else {
			procShowWindow.Call(hwnd, swShow)
		}
	}
	for _, hwnd := range historyControls {
		if show {
			procShowWindow.Call(hwnd, swShow)
		} else {
			procShowWindow.Call(hwnd, swHide)
		}
	}
}

func boolToUintptr(v bool) uintptr {
	if v {
		return 1
	}
	return 0
}

func buildUI(hwnd uintptr) {
	title := createControl("STATIC", "水滴记录", ssLeft, 24, 18, 260, 42, hwnd, 0)
	procSendMessageW.Call(title, wmSetFont, fontTitle, 1)
	createControl("STATIC", time.Now().Format("2006 年 1 月 2 日"), ssLeft, 24, 60, 250, 24, hwnd, 0)
	createControl("STATIC", "💧", ssCenter, 350, 24, 34, 34, hwnd, 0)

	progressBar = createControl("msctls_progress32", "", 0, 24, 98, 360, 22, hwnd, 0)
	totalLabel = createControl("STATIC", "", ssLeft, 24, 132, 250, 28, hwnd, 0)
	goalLabel = createControl("STATIC", "", ssLeft, 24, 160, 250, 24, hwnd, 0)
	remaining = createControl("STATIC", "", ssLeft, 276, 132, 108, 28, hwnd, 0)

	amounts := []int{150, 250, 350, 500}
	for i, amount := range amounts {
		createControl("BUTTON", fmt.Sprintf("+\r\n%d ml", amount), bsPushButton|bsMultiline|wsTabStop, int32(24+i*92), 202, 82, 58, hwnd, 101+i)
	}

	createControl("STATIC", "每日目标", ssLeft, 24, 282, 86, 26, hwnd, 0)
	goalEdit = createControl("EDIT", "", wsBorder|esNumber|wsTabStop, 116, 278, 120, 30, hwnd, 0)
	createControl("STATIC", "ml", ssLeft, 244, 284, 28, 24, hwnd, 0)
	createControl("BUTTON", "保存", bsPushButton|wsTabStop, 286, 278, 98, 30, hwnd, 120)

	createControl("BUTTON", "今日", bsPushButton|wsTabStop, 24, 326, 176, 34, hwnd, 130)
	createControl("BUTTON", "历史日历", bsPushButton|wsTabStop, 208, 326, 176, 34, hwnd, 131)

	todayTitle := createControl("STATIC", "今日记录", ssLeft, 24, 382, 180, 26, hwnd, 0)
	todayList = createControl("LISTBOX", "", wsBorder|wsVScroll|lbsNotify, 24, 414, 360, 232, hwnd, 0)
	deleteButton := createControl("BUTTON", "删除选中记录", bsPushButton|wsTabStop, 244, 660, 140, 32, hwnd, 140)
	todayControls = []uintptr{todayTitle, todayList, deleteButton}

	prev := createControl("BUTTON", "‹", bsPushButton|wsTabStop, 24, 378, 42, 30, hwnd, 150)
	monthLabel = createControl("STATIC", "", ssCenter, 76, 382, 256, 26, hwnd, 0)
	next := createControl("BUTTON", "›", bsPushButton|wsTabStop, 342, 378, 42, 30, hwnd, 151)
	historyControls = []uintptr{prev, monthLabel, next}

	weekdays := []string{"一", "二", "三", "四", "五", "六", "日"}
	for i, weekday := range weekdays {
		label := createControl("STATIC", weekday, ssCenter, int32(24+i*52), 420, 48, 22, hwnd, 0)
		historyControls = append(historyControls, label)
	}
	for row := 0; row < 6; row++ {
		for col := 0; col < 7; col++ {
			i := row*7 + col
			cell := createControl("STATIC", "", ssCenter|wsBorder, int32(24+col*52), int32(448+row*48), 48, 44, hwnd, 0)
			calendarCells[i] = cell
			historyControls = append(historyControls, cell)
		}
	}

	refreshAll()
	showHistory(false)
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmCreate:
		mainWin = hwnd
		buildUI(hwnd)
		return 0
	case wmCommand:
		id := int(wParam & 0xffff)
		switch id {
		case 101, 102, 103, 104:
			addWater([]int{150, 250, 350, 500}[id-101])
		case 120:
			if goal, err := strconv.Atoi(getText(goalEdit)); err == nil {
				if goal < 500 {
					goal = 500
				}
				if goal > 10000 {
					goal = 10000
				}
				data.Goal = goal
				saveData()
				refreshAll()
			}
		case 130:
			showHistory(false)
		case 131:
			showHistory(true)
			refreshCalendar()
		case 140:
			sel, _, _ := procSendMessageW.Call(todayList, lbGetCurSel, 0, 0)
			if sel != lbErr && int(sel) < len(todayEntryIDs) {
				idToDelete := todayEntryIDs[int(sel)]
				kept := data.Entries[:0]
				for _, entry := range data.Entries {
					if entry.ID != idToDelete {
						kept = append(kept, entry)
					}
				}
				data.Entries = kept
				saveData()
				refreshAll()
			}
		case 150:
			displayedMonth = displayedMonth.AddDate(0, -1, 0)
			refreshCalendar()
		case 151:
			next := displayedMonth.AddDate(0, 1, 0)
			now := time.Now()
			if next.Year() < now.Year() || (next.Year() == now.Year() && next.Month() <= now.Month()) {
				displayedMonth = next
				refreshCalendar()
			}
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func main() {
	loadData()
	now := time.Now()
	displayedMonth = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)

	icc := initCommonControlsEx{DwSize: uint32(unsafe.Sizeof(initCommonControlsEx{})), DwICC: iccProgress}
	procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))

	hInstance, _, _ = procGetModuleHandleW.Call(0)
	fontUI = createFont(-17, 400)
	fontTitle = createFont(-30, 700)

	className := utf16Ptr("WaterDropWindowClass")
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	icon, _, _ := procLoadIconW.Call(hInstance, 1)
	wc := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     hInstance,
		HIcon:         icon,
		HCursor:       cursor,
		HbrBackground: colorWindow + 1,
		LpszClassName: className,
		HIconSm:       icon,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowExW.Call(
		wsExTopMost,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16Ptr("水滴记录"))),
		wsOverlapped|wsVisible,
		0x80000000, 0x80000000, 430, 770,
		0, 0, hInstance, 0,
	)
	procShowWindow.Call(hwnd, swShow)
	procUpdateWindow.Call(hwnd)

	var msg message
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}
