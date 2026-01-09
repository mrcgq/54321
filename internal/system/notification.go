package system

import (
	"fmt"
	"os/exec"
	"runtime"
)

// =============================================================================
// 系统通知
// =============================================================================

// NotificationManager 通知管理器
type NotificationManager struct {
	appName string
}

// NewNotificationManager 创建通知管理器
func NewNotificationManager(appName string) *NotificationManager {
	return &NotificationManager{
		appName: appName,
	}
}

// Show 显示系统通知
func (n *NotificationManager) Show(title, message string) error {
	switch runtime.GOOS {
	case "windows":
		return n.showWindows(title, message)
	case "darwin":
		return n.showMacOS(title, message)
	case "linux":
		return n.showLinux(title, message)
	default:
		return fmt.Errorf("不支持的操作系统")
	}
}

// showWindows Windows通知（使用PowerShell）
func (n *NotificationManager) showWindows(title, message string) error {
	script := fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

$template = @"
<toast>
    <visual>
        <binding template="ToastText02">
            <text id="1">%s</text>
            <text id="2">%s</text>
        </binding>
    </visual>
</toast>
"@

$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($template)
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("%s").Show($toast)
`, title, message, n.appName)

	cmd := exec.Command("powershell", "-Command", script)
	return cmd.Run()
}

// showMacOS macOS通知
func (n *NotificationManager) showMacOS(title, message string) error {
	script := fmt.Sprintf(`display notification "%s" with title "%s"`, message, title)
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Run()
}

// showLinux Linux通知
func (n *NotificationManager) showLinux(title, message string) error {
	cmd := exec.Command("notify-send", title, message)
	return cmd.Run()
}
