package database

// LikeOperator requests a contains match. It can only be constructed through
// Like, so ordinary strings passed as filter values always remain literals.
type LikeOperator struct {
	value string
}

// Like matches rows whose column contains s. SQL wildcard characters in s
// keep their LIKE meaning because the caller explicitly selected this operator.
func Like(s string) LikeOperator {
	return LikeOperator{value: s}
}

// OrOperator marks one filter as an alternative. All alternatives in a filter
// map are grouped together, and that group is ANDed with every ordinary filter.
type OrOperator struct {
	value any
}

// Or marks value as part of the query's grouped alternatives. Value may be a
// literal, a slice (IN), or a value returned by Like.
func Or(value any) OrOperator {
	return OrOperator{value: value}
}

type AndOperator struct {
	value any
}

func And(value any) AndOperator {
	return AndOperator{value: value}
}

// Direction is an allowed SQL ordering direction.
type Direction uint8

const (
	Ascending Direction = iota + 1
	Descending
)

// Ordering is a schema-validated ordering term. Construct one with OrderBy.
type Ordering struct {
	column    string
	direction Direction
}

// OrderBy constructs an ordering term. The column and direction are validated
// against the queried GORM schema when the query is built.
func OrderBy(column string, direction Direction) Ordering {
	return Ordering{column: column, direction: direction}
}
