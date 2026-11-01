package service

import (
	"slices"
	"testing"

	"schooldiarysvu/internal/api"
)

func TestChildMatchReasons(t *testing.T) {
	t.Parallel()
	both := []api.ChildSuggestionReasons{api.Surname, api.Patronymic}
	surname := []api.ChildSuggestionReasons{api.Surname}
	patronymic := []api.ChildSuggestionReasons{api.Patronymic}
	tests := []struct {
		name, parent, student string
		want                  []api.ChildSuggestionReasons
	}{
		{"father with the same surname", "Иванов Владимир Петрович", "Иванов Пётр Владимирович", both},
		{"daughter of the father", "Иванов Владимир Петрович", "Иванова Анна Владимировна", both},
		{"mother: surname in the feminine form", "Петровская Ольга Ивановна", "Петровский Олег Сергеевич", surname},
		{"mother and son -ова/-ов", "Смирнова Мария Олеговна", "Смирнов Глеб Андреевич", surname},
		{"adjective surname", "Толстая Ирина Львовна", "Толстой Марк Ильич", surname},
		{"father with another surname", "Кузнецов Сергей Иванович", "Морозов Дмитрий Сергеевич", patronymic},
		{"name ending in -ий: Дмитрий → Дмитриевич", "Орлов Дмитрий Олегович", "Орлов Лев Дмитриевич", both},
		{"name ending in -ий with a soft sign: Василий → Васильевич", "Ким Василий Ли", "Ким Анна Васильевна", both},
		{"Юрий → Юрьевич", "Белов Юрий Павлович", "Белова Ева Юрьевна", both},
		{"Игорь → Игоревич", "Зайцев Игорь Андреевич", "Зайцев Тимур Игоревич", both},
		{"Павел → Павлович", "Соколов Павел Ильич", "Соколов Иван Павлович", both},
		{"Лев → Львович", "Гусев Лев Петрович", "Гусева Вера Львовна", both},
		{"Михаил → Михайлович", "Попов Михаил Юрьевич", "Попов Олег Михайлович", both},
		{"Илья → Ильич and Ильинична", "Егоров Илья Петрович", "Егорова Майя Ильинична", both},
		{"Никита → Никитич", "Лебедев Никита Олегович", "Лебедев Ян Никитич", both},
		{"Пётр with ё → Петрович", "Фролов Пётр Ильич", "Фролова Ксения Петровна", both},
		{"no relation", "Васильев Андрей Олегович", "Морозова Анна Игоревна", nil},
		{"a mother's name does not make a patronymic", "Сидорова Мария Ивановна", "Ершов Глеб Маратович", nil},
		{"student without a patronymic", "Ли Виктор Чен", "Ли Анна", surname},
		{"name of the father is not a prefix match", "Ершов Иван Олегович", "Котов Пётр Ивашкович", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := childMatchReasons(tt.parent, tt.student); !slices.Equal(got, tt.want) {
				t.Errorf("childMatchReasons(%q, %q) = %v, want %v", tt.parent, tt.student, got, tt.want)
			}
		})
	}
}
