package luhn

import (
	"fmt"
	"strings"
	"testing"
)

func TestValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		number string
		want   bool
	}{
		// Валидные номера
		{name: "простой валидный", number: "79927398713", want: true},
		{name: "номер из спецификации 1", number: "12345678903", want: true},
		{name: "номер из спецификации 2", number: "9278923470", want: true},
		{name: "номер из спецификации 3", number: "346436439", want: true},
		{name: "номер из спецификации withdraw", number: "2377225624", want: true},
		{name: "все нули", number: "00", want: true},

		// Невалидные номера
		{name: "контрольная цифра сбита", number: "79927398710", want: false},
		{name: "одна цифра", number: "0", want: false},
		{name: "пустая строка", number: "", want: false},
		{name: "буквы", number: "abc", want: false},
		{name: "смешано цифры и буквы", number: "1234a", want: false},
		{name: "пробелы", number: "7992 7398 713", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Valid(tc.number)
			if got != tc.want {
				t.Errorf("Valid(%q) = %v, want %v", tc.number, got, tc.want)
			}
		})
	}
}

// BenchmarkValid измеряет стоимость Valid на валидных номерах разной длины.
func BenchmarkValid(b *testing.B) {
	for _, n := range []int{10, 16, 19} {
		// Строка из нулей проходит Луна (сумма 0) и гоняет полный проход цикла.
		number := strings.Repeat("0", n)
		b.Run(fmt.Sprintf("len=%d", n), func(b *testing.B) {
			for b.Loop() {
				Valid(number)
			}
		})
	}
}
