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
	input := []interface{}{
		Person{
			Name: "Elmer",
			Age:  10,
		},
		User{
			ID:  101,
			UID: "ABCDE",
		},
	}

	q, err := New()
	g.Expect(err).To(gomega.BeNil())
	defer q.Close()

	for i := 0; i < len(input); i++ {
		err := q.Put(input[i])
		g.Expect(err).To(gomega.BeNil())
	}
	for i := 0; i < len(input); i++ {
		object, end, err := q.Next()
		g.Expect(object).ToNot(gomega.BeNil())
		g.Expect(err).To(gomega.BeNil())
		g.Expect(end).To(gomega.BeFalse())
	}
}
