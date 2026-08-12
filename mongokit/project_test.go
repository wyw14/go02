package mongokit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/256dpi/lungo/bsonkit"
)

func projectTest(t *testing.T, doc bson.M, fn func(fn func(bson.M, interface{}))) {
	t.Run("Mongo", func(t *testing.T) {
		coll := testCollection()
		res, err := coll.InsertOne(nil, doc)
		assert.NoError(t, err)

		fn(func(projection bson.M, result interface{}) {
			var out bson.M
			err := coll.FindOne(nil, bson.M{
				"_id": res.InsertedID,
			}, options.FindOne().SetProjection(projection)).Decode(&out)
			if _, ok := result.(string); ok {
				assert.Error(t, err)
				assert.Zero(t, out)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, result, out)
			}
		})
	})

	t.Run("Lungo", func(t *testing.T) {
		fn(func(projection bson.M, result interface{}) {
			res, err := Project(bsonkit.MustConvert(doc), bsonkit.MustConvert(projection))
			if str, ok := result.(string); ok {
				assert.Error(t, err)
				assert.Equal(t, str, err.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, bsonkit.MustConvert(result.(bson.M)), res)
			}
		})
	})
}

func TestProject(t *testing.T) {
	id := primitive.NewObjectID()

	// hide id
	projectTest(t, bson.M{
		"_id": id,
		"foo": "bar",
		"bar": "baz",
	}, func(fn func(bson.M, interface{})) {
		fn(bson.M{
			"_id": 0,
		}, bson.M{
			"foo": "bar",
			"bar": "baz",
		})
	})

	// include
	projectTest(t, bson.M{
		"_id": id,
		"foo": "bar",
		"bar": "baz",
	}, func(fn func(bson.M, interface{})) {
		fn(bson.M{
			"foo": 1,
		}, bson.M{
			"_id": id,
			"foo": "bar",
		})
	})

	// include, hide id
	projectTest(t, bson.M{
		"_id": id,
		"foo": "bar",
		"bar": "baz",
	}, func(fn func(bson.M, interface{})) {
		fn(bson.M{
			"_id": 0,
			"foo": 1,
		}, bson.M{
			"foo": "bar",
		})
	})

	// exclude
	projectTest(t, bson.M{
		"_id": id,
		"foo": "bar",
		"bar": "baz",
	}, func(fn func(bson.M, interface{})) {
		fn(bson.M{
			"foo": 0,
		}, bson.M{
			"_id": id,
			"bar": "baz",
		})
	})

	// exclude, hide id
	projectTest(t, bson.M{
		"_id": id,
		"foo": "bar",
		"bar": "baz",
	}, func(fn func(bson.M, interface{})) {
		fn(bson.M{
			"_id": 0,
			"foo": 0,
		}, bson.M{
			"bar": "baz",
		})
	})

	// exclude multiple fields
	projectTest(t, bson.M{
		"_id": id,
		"foo": "bar",
		"bar": "baz",
		"qux": "quux",
	}, func(fn func(bson.M, interface{})) {
		fn(bson.M{
			"foo": 0,
			"bar": 0,
		}, bson.M{
			"_id": id,
			"qux": "quux",
		})
	})

	// boolean true/false are equivalent to numeric 1/0
	projectTest(t, bson.M{
		"_id": id,
		"foo": "bar",
		"bar": "baz",
	}, func(fn func(bson.M, interface{})) {
		fn(bson.M{
			"foo": true,
		}, bson.M{
			"_id": id,
			"foo": "bar",
		})
		fn(bson.M{
			"foo": false,
		}, bson.M{
			"_id": id,
			"bar": "baz",
		})
	})

	// TODO: Test allowed mixing with _id.

	// mixed projection
	projectTest(t, bson.M{
		"_id": id,
		"foo": "bar",
		"bar": "baz",
	}, func(fn func(bson.M, interface{})) {
		fn(bson.M{
			"foo": 0,
			"bar": 1,
		}, "cannot have a mix of inclusion and exclusion")
	})
}

func TestProjectSlice(t *testing.T) {
	id := primitive.NewObjectID()

	projectTest(t, bson.M{
		"_id": id,
		"foo": bson.A{
			bson.M{
				"a": 1.0,
			},
			bson.M{
				"a": 2.0,
			},
			bson.M{
				"a": 3.0,
			},
		},
	}, func(fn func(bson.M, interface{})) {
		// zero
		fn(bson.M{
			"foo": bson.M{
				"$slice": 0,
			},
		}, bson.M{
			"_id": id,
			"foo": bson.A{},
		})

		// first one
		fn(bson.M{
			"foo": bson.M{
				"$slice": 1,
			},
		}, bson.M{
			"_id": id,
			"foo": bson.A{
				bson.M{
					"a": 1.0,
				},
			},
		})

		// first two
		fn(bson.M{
			"foo": bson.M{
				"$slice": 2,
			},
		}, bson.M{
			"_id": id,
			"foo": bson.A{
				bson.M{
					"a": 1.0,
				},
				bson.M{
					"a": 2.0,
				},
			},
		})

		// last one
		fn(bson.M{
			"foo": bson.M{
				"$slice": -1,
			},
		}, bson.M{
			"_id": id,
			"foo": bson.A{
				bson.M{
					"a": 3.0,
				},
			},
		})

		// last two
		fn(bson.M{
			"foo": bson.M{
				"$slice": -2,
			},
		}, bson.M{
			"_id": id,
			"foo": bson.A{
				bson.M{
					"a": 2.0,
				},
				bson.M{
					"a": 3.0,
				},
			},
		})

		// overload positive
		fn(bson.M{
			"foo": bson.M{
				"$slice": 5,
			},
		}, bson.M{
			"_id": id,
			"foo": bson.A{
				bson.M{
					"a": 1.0,
				},
				bson.M{
					"a": 2.0,
				},
				bson.M{
					"a": 3.0,
				},
			},
		})

		// overload negative
		fn(bson.M{
			"foo": bson.M{
				"$slice": -5,
			},
		}, bson.M{
			"_id": id,
			"foo": bson.A{
				bson.M{
					"a": 1.0,
				},
				bson.M{
					"a": 2.0,
				},
				bson.M{
					"a": 3.0,
				},
			},
		})
	})

	// $slice alone preserves all other fields (regression: previously
	// dropped every field except _id and the sliced one)
	projectTest(t, bson.M{
		"_id": id,
		"foo": bson.A{int32(1), int32(2), int32(3), int32(4)},
		"bar": "baz",
		"qux": int32(7),
	}, func(fn func(bson.M, interface{})) {
		fn(bson.M{
			"foo": bson.M{"$slice": 2},
		}, bson.M{
			"_id": id,
			"foo": bson.A{int32(1), int32(2)},
			"bar": "baz",
			"qux": int32(7),
		})
	})
}
