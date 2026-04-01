package pkg

type IoColor string

const (
	IoColorBlack   IoColor = "\033[30m"
	IoColorRed     IoColor = "\033[31m"
	IoColorGreen   IoColor = "\033[32m"
	IoColorYellow  IoColor = "\033[33m"
	IoColorBlue    IoColor = "\033[34m"
	IoColorPurple  IoColor = "\033[35m"
	IoColorCyan    IoColor = "\033[36m"
	IoColorDefault IoColor = "\033[97m"
	IoColorReset   IoColor = "\033[0m"
)

const (
	DefaultErrReportFileName string = "error.txt"
	DefaultRegion            string = "Asia/Jakarta"
	DefaultErrCodeLength     int    = 7
)
