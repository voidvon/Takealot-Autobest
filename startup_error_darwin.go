package main

import "os/exec"

func showStartupError(message string) {
	_ = exec.Command("osascript", "-e", "on run argv\ndisplay alert \"Takealot 掌柜启动失败\" message (item 1 of argv) as critical\nend run", message).Run()
}
