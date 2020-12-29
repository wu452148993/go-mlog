package transpiler

import (
	"context"
	"fmt"
)

func MLOGToString(ctx context.Context, statements [][]Resolvable, statement MLOGAble, lineNumber int) string {
	result := ""
	for _, line := range statements {
		resultLine := ""
		for _, t := range line {
			if resultLine != "" {
				resultLine += " "
			}
			resultLine += t.GetValue()
		}

		if ctx.Value(contextOptions).(Options).Numbers {
			result += fmt.Sprintf("%3d: ", lineNumber)
		}

		if ctx.Value(contextOptions).(Options).Comments {
			result += fmt.Sprintf("%-45s", resultLine)
			result += " // " + statement.GetComment()
		} else {
			result += resultLine
		}

		result += "\n"
		lineNumber++
	}
	return result
}
