package pretty

import (
	"encoding/json"
	"fmt"
	"time"
)

type PrettyPrintOption func(p *prettyPrinter)

type PrettyPrinter interface {
	Print(s string)
	Printf(format string, a ...any)
	PrettyLogInfoStringf(format string, a ...any)
	PrettyLogString(s string)
	PrintWarning(s string)
	PrintWarningf(format string, a ...any)
	PrintError(s string)
	PrintErrorf(format string, a ...any)
	PrettyPrintDateTime(time.Time)
	PrettyPrintTime(time.Time)
	PrettyPrintDate(time.Time)
	DateTimeSting(time.Time) string
}

type prettyPrinter struct {
	InfoColor int32 `json:"infoColor"`
	WarnColor int32 `json:"warnColor"`
	ErrColor  int32 `json:"errorColor"`
}

func WithInfoColor(c int32) PrettyPrintOption {
	return func(p *prettyPrinter) {
		p.InfoColor = c
	}
}

func WithWarnColor(c int32) PrettyPrintOption {
	return func(p *prettyPrinter) {
		p.WarnColor = c
	}
}

func WithErrColor(c int32) PrettyPrintOption {
	return func(p *prettyPrinter) {
		p.ErrColor = c
	}
}

func NewPrettyPrinter(opts ...PrettyPrintOption) *prettyPrinter {
	const (
		infoColor = int32(92)
		warnColor = int32(93)
		errColor  = int32(91)
	)

	printer := &prettyPrinter{
		InfoColor: infoColor,
		WarnColor: warnColor,
		ErrColor:  errColor,
	}
	for _, opt := range opts {
		opt(printer)
	}
	return printer
}

func (p *prettyPrinter) Print(s ...any) {
	fmt.Printf("\x1b[1;%dm%s\x1b[0m\n", p.InfoColor, s)
}

func (p *prettyPrinter) Printf(format string, a ...any) {
	fstring := fmt.Sprintf(format, a...)
	p.Print(fstring)
}

func (p *prettyPrinter) PrettyLogInfoStringf(s string, a ...any) string {
	formatted := fmt.Sprintf(s, a...)
	prettyFormatted := fmt.Sprintf("\x1b[1;%dm%s\x1b[0m\n", p.InfoColor, formatted)
	return prettyFormatted
}

func (p *prettyPrinter) PrettyLogInfoString(s string) string {
	prettyString := fmt.Sprintf("\x1b[1;%dm%s\x1b[0m\n", p.InfoColor, s)
	return prettyString
}

func (p *prettyPrinter) PrintWarning(s ...any) {
	fmt.Printf("\x1b[1;%dm%s\x1b[0m\n", p.WarnColor, s)
}

func (p *prettyPrinter) PrintWarningf(format string, a ...any) {
	fstring := fmt.Sprintf(format, a...)
	p.PrintWarning(fstring)
}

func (p *prettyPrinter) PrintError(s ...any) {
	fmt.Printf("\x1b[1;%dm%s\x1b[0m\n", p.ErrColor, s)
}

func (p *prettyPrinter) PrintErrorf(format string, a ...any) {
	fstring := fmt.Sprintf(format, a...)
	p.PrintError(fstring)
}

func (p *prettyPrinter) PrettyPrintDateTime(t time.Time) {
	dateTimeString := fmt.Sprintf("%d-%02d-%02dT%02d:%02d:%02d",
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second())
	p.Print(dateTimeString)
}

func (p *prettyPrinter) PrettyPrintDate(t time.Time) {
	dateTimeString := fmt.Sprintf("%d-%02d-%02d",
		t.Year(), t.Month(), t.Day())
	p.Print(dateTimeString)
}

func (p *prettyPrinter) PrettyPrintTime(t time.Time) {
	dateTimeString := fmt.Sprintf("%02d:%02d:%02d",
		t.Hour(), t.Minute(), t.Second())
	p.Print(dateTimeString)
}

func (p *prettyPrinter) DateTimeSting(t time.Time) string {
	dateTimeString := fmt.Sprintf("%d-%02d-%02dT%02d:%02d:%02d",
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second())
	return dateTimeString
}

func (p *prettyPrinter) PrettyPrintJson(data []byte) {
	Print("####### Json Data #######")
	response, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		p.PrintErrorf("Error marshaling response: %s", err.Error())
	}
	p.Print(string(response))
	fmt.Println()
}

func Print(s string) {
	const (
		infoColor = int32(92)
	)
	fmt.Printf("\x1b[1;%dm%s\x1b[0m\n", infoColor, s)
}

func Printf(format string, a ...any) {
	const (
		infoColor = int32(92)
	)
	fstring := fmt.Sprintf(format, a...)
	fmt.Printf("\x1b[1;%dm%s\x1b[0m\n", infoColor, fstring)
}

func PrettyString(s string, a ...any) string {
	const (
		infoColor = int32(92)
	)
	formatted := fmt.Sprintf(s, a...)
	prettyFormatted := fmt.Sprintf("\x1b[1;%dm%s\x1b[0m\n", infoColor, formatted)
	return prettyFormatted
}

func PrettyStringWithColor(s string, color int32, a ...any) string {
	formatted := fmt.Sprintf(s, a...)
	prettyFormatted := fmt.Sprintf("\x1b[1;%dm%s\x1b[0m", color, formatted)
	return prettyFormatted
}

func PrettyLogErrorString(s string) string {
	const (
		errorColor = int32(91)
	)
	prettyString := fmt.Sprintf("\x1b[1;%dm%s\x1b[0m", errorColor, s)
	return prettyString
}

func PrettyLogInfoString(s string) string {
	const (
		infoColor = int32(92)
	)
	prettyString := fmt.Sprintf("\x1b[1;%dm%s\x1b[0m", infoColor, s)
	return prettyString
}

func PrintWarning(s ...any) {
	const (
		warnColor = int32(93)
	)
	fmt.Printf("\x1b[1;%dm%s\x1b[0m\n", warnColor, s)
}

func PrintWarningf(format string, a ...any) {
	const (
		warnColor = int32(93)
	)
	fstring := fmt.Sprintf(format, a...)
	fmt.Printf("\x1b[1;%dm%s\x1b[0m\n", warnColor, fstring)
}

func PrintError(s ...any) {
	const (
		errColor = int32(91)
	)
	fmt.Printf("\x1b[1;%dm%s\x1b[0m\n", errColor, s)
}

func PrettyErrorLogString(format string, a ...any) string {
	fmtString := fmt.Sprintf(format, a...)

	const (
		errColor = int32(91)
	)
	logStr := fmt.Sprintf("\x1b[1;%dm%s\x1b[0m\n", errColor, fmtString)
	return logStr
}

func PrintErrorf(format string, a ...any) {
	fmtString := fmt.Sprintf(format, a...)

	const (
		errColor = int32(91)
	)
	fmt.Printf("\x1b[1;%dm%s\x1b[0m\n", errColor, fmtString)
}

func PrettyPrintDateTime(t time.Time) {
	dateTimeString := fmt.Sprintf("%d-%02d-%02dT%02d:%02d:%02d",
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second())
	Print(dateTimeString)
}

func PrettyPrintDate(t time.Time) {
	dateTimeString := fmt.Sprintf("%d-%02d-%02d",
		t.Year(), t.Month(), t.Day())
	Print(dateTimeString)
}

func PrettyPrintTime(t time.Time) {
	dateTimeString := fmt.Sprintf("%02d:%02d:%02d",
		t.Hour(), t.Minute(), t.Second())
	Print(dateTimeString)
}

func DateTimeSting(t time.Time) string {
	dateTimeString := fmt.Sprintf("%d-%02d-%02dT%02d:%02d:%02d",
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second())
	return dateTimeString
}

func PrettyPrintJson(data []byte) {
	Print("####### Json Data #######")
	response, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		PrintErrorf("Error marshaling response: %s", err.Error())
	}
	Print(string(response))
	fmt.Println()
}
