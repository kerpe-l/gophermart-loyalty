// Package luhn реализует проверку номеров заказов по алгоритму Луна.
package luhn

// Valid возвращает true, если строка number проходит проверку по алгоритму Луна.
// Строка должна содержать только цифры и быть длиной ≥ 2.
func Valid(number string) bool {
	if len(number) < 2 {
		return false
	}

	var sum int
	parity := len(number) % 2

	for i, ch := range number {
		if ch < '0' || ch > '9' {
			return false
		}
		digit := int(ch - '0')

		if i%2 == parity {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
	}

	return sum%10 == 0
}
