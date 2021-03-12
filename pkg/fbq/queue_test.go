package fbq

import (
	"github.com/onsi/gomega"
	"testing"
)

func TestQueue(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	type Person struct {
		Name string
		Age  int
	}
	type User struct {
		ID  int
		UID string
	}
	input := []interface{}{}
	for i := 0; i < 1000; i++ {
		input = append(
			input,
			Person{
				Name: "Elmer",
				Age:  i + 10,
			})
		input = append(
			input,
			User{
				ID:  i,
				UID: "ABCDE",
			})
	}

	q := New()
	defer q.Close(true)

	for i := 0; i < len(input); i++ {
		err := q.Put(input[i])
		g.Expect(err).To(gomega.BeNil())
	}
	for i := 0; i < len(input); i++ {
		object, end, err := q.Get()
		g.Expect(object).ToNot(gomega.BeNil())
		g.Expect(err).To(gomega.BeNil())
		g.Expect(end).To(gomega.BeFalse())
	}
	reader, err := q.NewReader()
	g.Expect(err).To(gomega.BeNil())
	for i := 0; i < len(input); i++ {
		object, end, err := reader.Get()
		g.Expect(object).ToNot(gomega.BeNil())
		g.Expect(err).To(gomega.BeNil())
		g.Expect(end).To(gomega.BeFalse())
	}
	reader.Close()
}
