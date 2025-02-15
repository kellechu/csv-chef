package recipe

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

func TestTransformation_ParseExecute(t *testing.T) {
	tests := []struct {
		name             string
		recipe           string
		input            string
		processHeader    bool
		want             string
		wantParseErr     bool
		wantParseErrText string
		wantErr          bool
		wantErrText      string
	}{
		{
			name:          "simple 1 <- 1",
			recipe:        "!1 <- 1\n1 <- 1\n",
			processHeader: true,
			input:         "a,b\n",
			want:          "a\n",
		},
		{
			name:        "empty recipe is a parse error",
			wantErr:     true,
			wantErrText: "no column recipes provided",
		},
		{
			name:          "referencing a header for a column that has no reference is an error",
			recipe:        "1 <- \"hi\"\n!3 <- \"lala\"",
			processHeader: true,
			wantErr:       true,
			wantErrText:   "found header for column 3, but no recipe for column 3",
		},
		{
			name:          "process headers with no header recipe",
			recipe:        "1<-2\n2<-1\n",
			input:         "a,b\n",
			processHeader: true,
			want:          "a,b\n",
		},
		{
			name:          "header recipe with literals",
			recipe:        "1<-1\n2<-2\n!2<-\"apple\"\n",
			input:         "a,b\n",
			processHeader: true,
			want:          "a,apple\n",
		},
		{
			name:          "header recipe with joining literals",
			recipe:        "!1<- \"alpha\"+\" beta\"\n1<-1\n2<-2\n",
			input:         "a,b\n",
			processHeader: true,
			want:          "alpha beta,b\n",
		},
		{
			name:          "double join flip flop headers",
			recipe:        "!1<-2+1\n!2<-1+2\n1<-1\n2<-2\n",
			input:         "alpha,beta\n",
			processHeader: true,
			want:          "betaalpha,alphabeta\n",
		},
		{
			name:          "header referencing variable that does not exist is an error",
			recipe:        "!1<-$bar\n1<-1\n",
			input:         "a,b\n",
			processHeader: true,
			wantErr:       true,
			wantErrText:   "line 1 / header 1: variable '$bar' referenced, but it is not defined",
		},
		{
			name:          "headers via variables",
			recipe:        "$foo<-2\n1<-$foo\n!1<-$foo\n",
			input:         "apple,banana\n",
			processHeader: true,
			want:          "banana\n",
		},
		{
			name:          "referencing header column that does not exist is an error",
			recipe:        "1 <- 1\n!1 <- 3\n",
			input:         "a,b\n",
			processHeader: true,
			wantErr:       true,
			wantErrText:   "line 1 / header 1: column 3 referenced, but it does not exist in the input",
		},
		{
			name:          "referencing variable that is not defined is an error",
			recipe:        "1<-1\n!1<-$foo\n",
			input:         "a,b",
			processHeader: true,
			wantErr:       true,
			wantErrText:   "line 1 / header 1: variable '$foo' referenced, but it is not defined",
		},
		{
			name:          "double header using placeholder concatenation",
			recipe:        "!1 <- 1 + ?\n1<-1\n",
			input:         "ab,c\n",
			processHeader: true,
			want:          "abab\n",
		},
		{
			name:          "quad header using placeholder concatenation",
			recipe:        "!1 <- 1 + ? + ?\n1<-1\n",
			input:         "ab,c\n",
			processHeader: true,
			want:          "abababab\n",
		},
		{
			name:          "headers and column recipe, swap columns",
			recipe:        "!1 <- \"col1\"\n!2<-\"col2\"\n1<-2\n2<-1",
			input:         "first,last\na,b\nc,d\ne,f",
			processHeader: true,
			want:          "col1,col2\nb,a\nd,c\nf,e\n",
		},
		{
			name:   "column recipe, more complex",
			recipe: "1 <- 3 + 2\n2 <- 1 + 3\n3 <- 2 + 1\n",
			input:  "a,b,c\nd,e,f\ng,h,i",
			want:   "cb,ac,ba\nfe,df,ed\nih,gi,hg\n",
		},
		{
			name:   "column recipe, same as above, but variables first",
			recipe: "$a <- 3+2\n$b<-1+3\n$c<-2+1\n1<-$a\n2<-$b\n3<-$c\n",
			input:  "a,b,c\nd,e,f\ng,h,i",
			want:   "cb,ac,ba\nfe,df,ed\nih,gi,hg\n",
		},
		{
			name:          "upper 1, lower 2 - function test #1",
			recipe:        "!1 <- \"FRUIT\"\n1 <- 1 -> uppercase\n!2 <- \"veggies\"\n2 <- 2 -> lowercase",
			input:         "thing1,thing2\napple,artichoke\nBANANA,BEET\nCucumber,Carrot\n",
			processHeader: true,
			want:          "FRUIT,veggies\nAPPLE,artichoke\nBANANA,beet\nCUCUMBER,carrot\n",
		},
		{
			name:          "same as above but not using placeholder",
			recipe:        "!1 <- \"FRUIT\"\n1 <- uppercase(1)\n!2 <- \"veggies\"\n2 <- lowercase(2)",
			input:         "thing1,thing2\napple,artichoke\nBANANA,BEET\nCucumber,Carrot\n",
			processHeader: true,
			want:          "FRUIT,veggies\nAPPLE,artichoke\nBANANA,beet\nCUCUMBER,carrot\n",
		},
		{
			name:   "using join as a pipe function",
			recipe: "1 <- 1 -> join -> 1",
			input:  "a\nb\n",
			want:   "aa\nbb\n",
		},
		{
			name:   "using join as a function",
			recipe: "1 <- 1 -> join(1)",
			input:  "a\nb\n",
			want:   "aa\nbb\n",
		},
		{
			name:   "using join as a function and joining to it",
			recipe: "1 <- 1 + join(1)",
			input:  "a\nb\n",
			want:   "aa\nbb\n",
		},
		{
			name:          "use add to sum two integer columns",
			recipe:        "!1 <- \"fruits\"\n!2 <- \"veggies\"\n!3 <- \"total\"\n1 <- 1\n2 <- 2\n3 <- add(1,2)",
			input:         "a,b\n1,2\n555,444\n13,31\n",
			processHeader: true,
			want:          "fruits,veggies,total\n1,2,3.000000\n555,444,999.000000\n13,31,44.000000\n",
		},
		{
			name:          "use addFloat to sum two float/int columns",
			recipe:        "!1 <- \"fruits\"\n!2 <- \"veggies\"\n!3 <- \"total\"\n1 <- 1\n2 <- 2\n3 <- add(1,2)\n",
			input:         "a,b\n1,2\n555.55,444.44\n13.55,31.44\n",
			processHeader: true,
			want:          "fruits,veggies,total\n1,2,3.000000\n555.55,444.44,999.990000\n13.55,31.44,44.990000\n",
		},
		{
			name:          "use addFloat to sum two float/int into rounded ints",
			recipe:        "!1 <- \"fruits\"\n!2 <- \"veggies\"\n!3 <- \"total\"\n1 <- 1\n2 <- 2\n3 <- add(1,2)\n",
			input:         "a,b\n1,2\n555.55,444.44\n13.55,31.44\n",
			processHeader: true,
			want:          "fruits,veggies,total\n1,2,3.000000\n555.55,444.44,999.990000\n13.55,31.44,44.990000\n",
		},
		{
			name:          "use addFloat to sum two float/int with no rounding",
			recipe:        "!1 <- \"fruits\"\n!2 <- \"veggies\"\n!3 <- \"total\"\n1 <- 1\n2 <- 2\n3 <- add(1,2)\n",
			input:         "a,b\n1,2\n555.55,444.44\n13.55,31.44\n",
			processHeader: true,
			want:          "fruits,veggies,total\n1,2,3.000000\n555.55,444.44,999.990000\n13.55,31.44,44.990000\n",
		},
		{
			name:        "add with non-int arg1 is an error",
			recipe:      "1 <- add(1, 2)\n",
			input:       "a,2\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: add(): first arg to Add was not numeric: a",
		},
		{
			name:        "add with non-int arg2 is an error",
			recipe:      "1 <- add(2,1)\n",
			input:       "a,2\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: add(): second arg to Add was not numeric: a",
		},
		{
			name:        "addFloat with non-int arg1 is an error",
			recipe:      "1 <- add(1, 2)\n",
			input:       "a,2\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: add(): first arg to Add was not numeric: a",
		},
		{
			name:        "addFloat with non-int arg2 is an error",
			recipe:      "1 <- add(2, 1, \"0\")\n",
			input:       "1,2\na,2\n",
			wantErr:     true,
			wantErrText: "line 2 / column 1: add(): second arg to Add was not numeric: a",
		},
		{
			name:        "join with column that does not exist is an error",
			recipe:      "1 <- 1 -> join(3)\n",
			input:       "a,b\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: column 3 referenced, but it does not exist in the input",
		},
		{
			name:        "uppercase with bad reference is an error",
			recipe:      "1 <- uppercase($foo)\n",
			input:       "a,b\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: uppercase(): error evaluating arg: variable '$foo' referenced, but it is not defined",
		},
		{
			name:        "lowercase with bad reference is an error",
			recipe:      "1 <- lowercase($bar)\n",
			input:       "a,b\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: lowercase(): error evaluating arg: variable '$bar' referenced, but it is not defined",
		},
		{
			name:        "add with bad reference is an error",
			recipe:      "1 <- add($bar, 1)\n",
			input:       "a,b\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: add(): error evaluating arg: variable '$bar' referenced, but it is not defined",
		},
		{
			name:        "addfloat with bad reference is an error",
			recipe:      "1 <- add(1,1)\n2<- add(2,3)\n",
			input:       "1,2.0\n",
			wantErr:     true,
			wantErrText: "line 1 / column 2: add(): error evaluating arg: column 3 referenced, but it does not exist in the input",
		},
		{
			name:          "chain of change calls",
			recipe:        "1 <- 1 -> change(\"acc\", \"accepted\") -> change(\"rej\", \"rejected\") -> change(\"mailed\", \"outbound\") -> uppercase",
			input:         "status\nacc\nrej\nmailed\nextra\n",
			processHeader: true,
			want:          "status\nACCEPTED\nREJECTED\nOUTBOUND\nEXTRA\n",
		},
		{
			name:        "change call with bad reference is an error",
			recipe:      "1 <- 1 -> change(\"foo\", $foo)",
			input:       "a,b\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: change(): error evaluating arg: variable '$foo' referenced, but it is not defined",
		},
		{
			name:          "chain of changeI calls",
			recipe:        "1 <- 1 -> changei(\"acc\", \"accepted\") -> changei(\"rej\", \"rejected\") -> changei(\"mailed\", \"outbound\") -> uppercase",
			input:         "Status\naCc\nREJ\nmAiled\nunmapped\n",
			processHeader: true,
			want:          "Status\nACCEPTED\nREJECTED\nOUTBOUND\nUNMAPPED\n",
		},
		{
			name:        "changeI call with bad reference is an error",
			recipe:      "1 <- 1 -> changei(\"foo\", $foo)",
			input:       "a,b\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: changei(): error evaluating arg: variable '$foo' referenced, but it is not defined",
		},
		{
			name:   "ifempty test",
			recipe: "1 <- 1 -> ifempty(\"EMPTY\", \"NOT\")\n2 <- 2 -> ifempty(3, \"!!\")\n",
			input:  ",,hi\na,,hi\n,b,hi\n",
			want:   "EMPTY,hi\nNOT,hi\nEMPTY,!!\n",
		},
		{
			name:   "isempty test (alias of ifempty)",
			recipe: "1 <- 1 -> isempty(\"EMPTY\", \"NOT\")\n2 <- 2 -> isempty(3, \"!!\")\n",
			input:  ",,hi\na,,hi\n,b,hi\n",
			want:   "EMPTY,hi\nNOT,hi\nEMPTY,!!\n",
		},
		{
			name:        "ifempty test with reference error",
			recipe:      "1 <- ifempty(\"EMPTY\", \"NOT\", $bar)\n",
			input:       ",,hi\na,,hi\n,b,hi\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: ifempty(): error evaluating arg: variable '$bar' referenced, but it is not defined",
		},
		{
			name:   "ifempty used to leave value alone",
			recipe: "1 <- 1 -> ifempty(\"empty\")",
			input:  ",lala\nA,a\nb,B\n",
			want:   "empty\nA\nb\n",
		},
		{
			name:   "test subtract",
			recipe: "1 <- subtract(2,3)",
			input:  "a,50,40\na,10,10\na,5,10\n",
			want:   "10.000000\n0.000000\n-5.000000\n",
		},
		{
			name:        "test subtract errors",
			recipe:      "1 <- subtract($foo,1)",
			input:       "1",
			wantErr:     true,
			wantErrText: "line 1 / column 1: subtract(): error evaluating arg: variable '$foo' referenced, but it is not defined",
		},
		{
			name:   "numberFormat can limit decimals on a number",
			recipe: "1 <- 1->numberFormat(\"2\")\n",
			input:  "46.2577000",
			want:   "46.26\n",
		},
		{
			name:        "numberFormat will error if input is not numeric",
			recipe:      "1 <- 1->numberFormat(\"2\")",
			input:       "2.3\nalpha\n",
			wantErr:     true,
			wantErrText: "line 2 / column 1: numberformat(): error: input is not numeric: got 'alpha'",
		},
		{
			name:        "numberFormat will error if digits parameter is not a whole number numeric",
			recipe:      "1 <- 1 -> numberFormat(2)",
			input:       "2.3,beta",
			wantErr:     true,
			wantErrText: "line 1 / column 1: numberformat(): error: digits must be an integer, got 'beta'",
		},
		{
			name:   "multiply returns the product of two numeric inputs",
			recipe: "1 <- multiply(1,2)\n",
			input:  "12,12\n4.5,3.0\n",
			want:   "144.000000\n13.500000\n",
		},
		{
			name:        "multiply return error if first arg is not numeric",
			recipe:      "1 <- multiply(\"abc\", 2)\n",
			input:       "12,12\n4.5,3.0\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: multiply(): error: first arg to multiply was not numeric, got 'abc'",
		},
		{
			name:        "multiply return error if second arg is not numeric",
			recipe:      "1 <- multiply(1, 2)\n",
			input:       "12,12\n4.5,def\n",
			wantErr:     true,
			wantErrText: "line 2 / column 1: multiply(): error: second arg to multiply was not numeric, got 'def'",
		},
		{
			name:   "divide provides the answer to dividing two numbers",
			recipe: "1 <- divide(1,2)\n",
			input:  "1000,100\n22,7\n",
			want:   "10.000000\n3.142857\n",
		},
		{
			name:   "test divide with numberFormat to provide the answer to dividing two numbers",
			recipe: "1 <- divide(1,2) -> numberFormat(\"2\")",
			input:  "1000,100\n22,7\n",
			want:   "10.00\n3.14\n",
		},
		{
			name:        "divide has an error if the first argument is not numeric",
			recipe:      "1 <- divide(1,2)\n",
			input:       "apple,5",
			wantErr:     true,
			wantErrText: "line 1 / column 1: divide(): error: first arg to divide was not numeric, got 'apple'",
		},
		{
			name:        "divide has an error if the second argument is not numeric",
			recipe:      "1 <- divide(1,2)\n",
			input:       "13.2,salami",
			wantErr:     true,
			wantErrText: "line 1 / column 1: divide(): error: second arg to divide was not numeric, got 'salami'",
		},
		{
			name:        "divide has an error if the second argument is zero",
			recipe:      "$foo <- subtract(1,2)\n1<-divide(1,$foo)\n",
			input:       "4,4\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: divide(): error: attempt to divide by zero",
		},
		{
			name:   "lineno returns the current line number",
			recipe: "1<-lineno\n2<-1",
			input:  "a\nb\nc\nd\n",
			want:   "1,a\n2,b\n3,c\n4,d\n",
		},
		{
			name:   "removeDigits removes any digits in an input",
			recipe: "1<-1->removeDigits\n",
			input:  "alpha,\n12345,\na1b2c3,\n",
			want:   "alpha\n\nabc\n",
		},
		{
			name:        "bad reference in removeDigits is an error",
			recipe:      "1<-removeDigits(32)\n",
			input:       "alpha,\n12345,\na1b2c3\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: removedigits(): error evaluating arg: column 32 referenced, but it does not exist in the input",
		},
		{
			name:   "onlyDigits leaves just the digits in an input",
			recipe: "1<-1->onlyDigits\n",
			input:  "alpha,\n12345,\na1b2c3,\n",
			want:   "\n12345\n123\n",
		},
		{
			name:        "bad reference in onlyDigits is an error",
			recipe:      "1<-onlyDigits(16)\n",
			input:       "alpha,\n12345,\na1b2c3\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: onlydigits(): error evaluating arg: column 16 referenced, but it does not exist in the input",
		},
		{
			name:   "mod function returns the remainder of dividing two ints",
			recipe: "1 <- mod(1,2)",
			input:  "0,2\n1,2\n2,2\n6,10\n",
			want:   "0\n1\n0\n6\n",
		},
		{
			name:        "mod function returns error if first arg is not int",
			recipe:      "1 <- mod(1, 2)",
			input:       "0,2\n3,4\napple,4\n5,10\n",
			wantErr:     true,
			wantErrText: "line 3 / column 1: mod(): first arg to mod was not an integer: 'apple'",
		},
		{
			name:        "mod function returns error if second arg is not int",
			recipe:      "1 <- mod(1, 2)",
			input:       "0,2\n3,4\n1,4\n5,banana\n",
			wantErr:     true,
			wantErrText: "line 4 / column 1: mod(): second arg to mod was not an integer: 'banana'",
		},
		{
			name:        "mod returns an error if divisor is zero",
			recipe:      "1 <- mod(1, 2)",
			input:       "0,2\n3,4\n2,0\n5,10\n",
			wantErr:     true,
			wantErrText: "line 3 / column 1: mod(): attempt to divide by zero",
		},
		{
			name:   "trim removes leading and trailing whitespace",
			recipe: "1 <- trim(1)\n2 <- 2 -> trim\n",
			input:  " apple , banana   \nartichoke  ,  kumquat\n   salad greens,squash the beef   \n",
			want:   "apple,banana\nartichoke,kumquat\nsalad greens,squash the beef\n",
		},
		{
			name:   "firstChars returns the first X characters of the input",
			recipe: "1 <- firstChars(\"3\", 1)\n",
			input:  "apple\nbanana\npear\nab\n世界世界世界\n世界\n",
			want:   "app\nban\npea\nab\n世界世\n世界\n",
		},
		{
			name:   "firstChars returns the first X characters of the input using implicit placeholder",
			recipe: "1 <- 1 -> firstChars(\"3\")\n",
			input:  "apple\nbanana\npear\n世界世界世界\n",
			want:   "app\nban\npea\n世界世\n",
		},
		{
			name:        "firstChars with non-int first parameter is an error",
			recipe:      "1 <- 1 -> firstChars(\"apple\")\n",
			input:       "apple\nbanana\npear\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: firstchars(): first arg is not an integer: got 'apple'",
		},
		{
			name:        "firstChars with negative first parameter is an error",
			recipe:      "1 <- 1 -> firstChars(\"-2\")\n",
			input:       "apple\nbanana\npear\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: firstchars(): first arg is negative: got '-2'",
		},
		{
			name:   "lastChars returns the last N characters of input",
			recipe: "1 <- 1 -> lastChars(\"3\")\n",
			input:  "scan\nscat\nfrat\nhalifax\n世界世界世界\n",
			want:   "can\ncat\nrat\nfax\n界世界\n",
		},
		{
			name:   "lastChars returns the whole input if N is larger than input size",
			recipe: "1 <- 1 -> lastChars(\"4\")\n",
			input:  "pan\nban\nman\n界世界\n",
			want:   "pan\nban\nman\n界世界\n",
		},
		{
			name:   "lastChars with dynamic size",
			recipe: "1 <- lastChars(1, 2)",
			input:  "4,scowl\n5,pineapple\n3,slap\n",
			want:   "cowl\napple\nlap\n",
		},
		{
			name:        "lastChars returns error if count param is non-int",
			recipe:      "1 <- lastChars(1, 2)",
			input:       "4,scowl\n5,pineapple\nfireball,larp\n",
			wantErr:     true,
			wantErrText: "line 3 / column 1: lastchars(): first arg is not an integer: got 'fireball'",
		},
		{
			name:        "lastChars returns error if count param is negative",
			recipe:      "1 <- lastChars(1, 2)",
			input:       "4,scowl\n5,pineapple\n-2,larp\n",
			wantErr:     true,
			wantErrText: "line 3 / column 1: lastchars(): first arg is negative: got '-2'",
		},
		{
			name:   "repeat repeats a string some number of times",
			recipe: "1 <- 1 -> repeat(\"2\")",
			input:  "la\nboo\nfefi\n",
			want:   "lala\nbooboo\nfefifefi\n",
		},
		{
			name:   "repeat repeats a string some number of times with explicit 2nd arg",
			recipe: "1 <- repeat(\"2\", 1)",
			input:  "la\nboo\nfefi\n",
			want:   "lala\nbooboo\nfefifefi\n",
		},
		{
			name:   "silly repeat test",
			recipe: "1 <- 1 -> repeat",
			input:  "1\n2\n3\n4\n5\n",
			want:   "1\n22\n333\n4444\n55555\n",
		},
		{
			name:        "repeat has an error if first argument is not an integer",
			recipe:      "1 <- repeat(\"abc\", \"foo\")",
			input:       "a\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: repeat(): first arg is not an integer: got 'abc'",
		},
		{
			name:        "repeat has an error if first argument is negative",
			recipe:      "1 <- repeat(1)\n",
			input:       "2\n4\n-4\n",
			wantErr:     true,
			wantErrText: "line 3 / column 1: repeat(): first arg is negative: got '-4'",
		},
		{
			name:   "replace will turn found search to something else in the input",
			recipe: "1 <- 1 -> replace(\"ab\",\"Co\")",
			input:  "abla\nabde\nabop\nsalad\nabca-abla\n",
			want:   "Cola\nCode\nCoop\nsalad\nCoca-Cola\n",
		},
		{
			name:   "today returns today's date in Y-m-d format",
			recipe: "1 <- 1\n2 <- today",
			input:  "a\nb\n",
			want:   "a,2021-08-30\nb,2021-08-30\n",
		},
		{
			name:   "now returns current date and time in RFC 3339 format",
			recipe: "1 <- now",
			input:  "a\n",
			want:   "2021-08-30T18:22:13-06:00\n",
		},
		{
			name:   "formatDate can return dates formatted as requested",
			recipe: "1 <- now -> formatDate(1)\n",
			input:  "2006-01-02\nAmerica/Denver\n\"Mon Jan 2, 2006 3:04:05 pm\"\nham\n",
			want:   "2021-08-30\nAmerica/Denver\n\"Mon Aug 30, 2021 6:22:13 pm\"\nham\n",
		},
		{
			name:        "formatDateF will return error if format unknown",
			recipe:      "1 <- now -> formatDate(\"2006-01-02\") -> formatDateF(1)\n",
			input:       "2006-01-02\nAmerica/Denver\n\"Mon Jan 2, 2006 3:04:05 pm\"\nham\n",
			wantErr:     true,
			wantErrText: "line 1 / column 1: formatdatef(): expected RFC3339 format for input date: '2021-08-30'",
		},
		{
			name:   "formatDate called with non-date passes input through",
			recipe: "1 <- 1 -> formatDate(\"2005-04-03\")",
			input:  "a\n",
			want:   "a\n",
		},
		{
			name:   "formatDate with good date",
			recipe: "1 <- formatDate(\"2006-01-02\", 1)\n",
			input:  "2021-08-30T18:22:13-06:00\n1977-08-23T19:49:00-06:00",
			want:   "2021-08-30\n1977-08-23\n",
		},
		{
			name:   "readDate will read dates with a given format",
			recipe: "1 <- 1 -> readDate(2) -> formatDate(\"2006-01-02 15:04:05\")\n",
			input:  "\"Oct 31, 2022\",\"Jan 02, 2006\"\n\"05-09-80\",\"01-02-06\"\n\"01-01-1970\",\"01-02-2006\"\n\"Feb 3, 2004 16:55 MST\",\"Jan 2, 2006 15:04 MST\"\n",
			want:   "2022-10-31 00:00:00\n1980-05-09 00:00:00\n1970-01-01 00:00:00\n2004-02-03 16:55:00\n",
		},
		{
			name:   "readDate will pass the input unchanged if it's not recognized",
			recipe: "1 <- 1 -> readDate(\"2006-01-02\") -> readDate(\"1/2/2006\") -> formatDate(\"Jan 2 2006\")",
			input:  "2021-04-14\n5/6/2019\nbanana\n",
			want:   "Apr 14 2021\nMay 6 2019\nbanana\n",
		},
		{
			name:        "readDateF will have an error if format is not recognized",
			recipe:      "1 <- 1 -> readDateF(\"2006-01-02\")",
			input:       "2021-04-14\n5/6/2019\nbanana\n",
			wantErr:     true,
			wantErrText: "line 2 / column 1: readdatef(): unrecognized date '5/6/2019' for format: '2006-01-02'",
			want:        "Apr 14 2021\nMay 6 2019\nbanana\n",
		},
		{
			name:   "smartDate reads dates... smartly.",
			recipe: "1 <- smartDate(1) -> formatDate(\"2006-01-02\")\n",
			input:  "5/6/1980\n03-02-2004\n\"Dec 25, 1980\"\n1942-06-12\n",
			want:   "1980-05-06\n2004-02-03\n1980-12-25\n1942-06-12\n",
		},
		{
			name:   "isPast returns a value if the date is in the past",
			recipe: "1 <- 1->isPast(\"past\", \"future\")",
			input:  "1/1/1980\n\"August 30, 2021 08:00:00-06:00\"\n2021/08/31 12:00:00-06:00\n\"Jan 4, 2022\"\n",
			want:   "past\npast\nfuture\nfuture\n",
		},
		{
			name:   "isFuture returns a value if the date is in the future",
			recipe: "1 <- 1->isFuture(\"future\", \"past\")",
			input:  "1/1/1980\n\"August 30, 2021 08:00:00-06:00\"\n2021/08/31 12:00:00-06:00\n\"Jan 4, 2022\"\n",
			want:   "past\npast\nfuture\nfuture\n",
		},
		{
			name:   "isPast will leave non-date input untouched",
			recipe: "1 <- 1 -> readDate(\"2006-01-02\") -> isPast(\"SENT\", \"UNSENT\")",
			input:  "2021-07-04,\n,\n2021-08-30,\n2021-08-31,\n2022-01-01,\n",
			want:   "SENT\n\nSENT\nSENT\nUNSENT\n",
		},
		{
			name:   "readDate will leave unrecognized input untouched",
			recipe: "1 <- 1 -> readDate(\"2006-01-02\")\n2<-2\n",
			input:  "2021-07-04,\n,\n2021-08-30,\n2021-08-31,\n2022-01-01,\n",
			want:   "2021-07-04T00:00:00Z,\n,\n2021-08-30T00:00:00Z,\n2021-08-31T00:00:00Z,\n2022-01-01T00:00:00Z,\n",
		},
		{
			name:          "columns are automatically named if not provided in source",
			recipe:        "1<-1\n2<-1\n3<-1\n",
			input:         "header\na\nb\n",
			processHeader: true,
			want:          "header,column 2,column 3\na,a,a\nb,b,b\n",
		},
		{
			name:          "Blank lines in recipes are ignored",
			recipe:        "# this is a comment and then there's a blank line\n\n!1 <- 1 # voter_id header\n1 <- 1 # voter_id\n!2 <- 2 # first header\n2 <- 2 # first\n",
			input:         "header1,header2\na,b\nc,d\n",
			processHeader: true,
			want:          "header1,header2\na,b\nc,d\n",
		},
		{
			name:          "Whitespace lines in recipes are ignored",
			recipe:        "# this is a comment and then there's a whitespace line\n             \n!1 <- 1 # voter_id header\n1 <- 1 # voter_id\n!2 <- 2 # first header\n2 <- 2 # first\n",
			input:         "header1,header2\na,b\nc,d\n",
			processHeader: true,
			want:          "header1,header2\na,b\nc,d\n",
		},
		{
			name:             "column can only be defined once",
			recipe:           "1 <- 1\n1<-1\n",
			wantParseErr:     true,
			wantParseErrText: "error - line 2: column 1 already defined",
		},
		{
			name:             "header can only be defined once",
			recipe:           "!1 <- 1\n#\n#\n!1<-1\n",
			wantParseErr:     true,
			wantParseErrText: "error - line 4: header 1 already defined",
		},
		{
			name:             "variable can only be defined once",
			recipe:           "$foo <- 1\n#\n#\n#\n$foo<-2\n",
			wantParseErr:     true,
			wantParseErrText: "error - line 5: variable $foo already defined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformation, err := Parse(strings.NewReader(tt.recipe))

			if (err != nil) != tt.wantParseErr {
				t.Errorf("parse error = %v, wantErr %v", err, tt.wantParseErrText)
				return
			}
			if tt.wantParseErr && err.Error() != tt.wantParseErrText {
				t.Errorf("got parse error text = %v, want error text = %v", err.Error(), tt.wantParseErrText)
				return
			}
			if tt.wantParseErr {
				return
			}

			var b bytes.Buffer
			writer := csv.NewWriter(&b)

			// Provide fixed implementation of Now so time test can work
			Now = func() time.Time {
				loc, _ := time.LoadLocation("America/Denver")
				return time.Date(2021, 8, 30, 18, 22, 13, 4445788, loc)
			}

			_, err = transformation.Execute(csv.NewReader(strings.NewReader(tt.input)), writer, tt.processHeader, -1)
			if (err != nil) != tt.wantErr {
				t.Errorf("execute error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && (err != nil) && err.Error() != tt.wantErrText {
				t.Errorf("got execute error text = %v, want error text = %v", err.Error(), tt.wantErrText)
				return
			}
			if tt.wantErr {
				// Leave test here because if we're testing for an error, we aren't testing for output
				return
			}

			got := b.String()
			if got != tt.want {
				t.Errorf("Dump() = %v, want %v", got, tt.want)
			}
		})
	}
}
