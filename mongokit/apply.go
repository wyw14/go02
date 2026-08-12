package mongokit

import (
	"fmt"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/256dpi/lungo/bsonkit"
)

// FieldUpdateOperators defines the field update operators.
var FieldUpdateOperators = map[string]Operator{}

func init() {
	// register field update operators
	FieldUpdateOperators["$set"] = applySet
	FieldUpdateOperators["$setOnInsert"] = applySetOnInsert
	FieldUpdateOperators["$unset"] = applyUnset
	FieldUpdateOperators["$rename"] = applyRename
	FieldUpdateOperators["$inc"] = applyInc
	FieldUpdateOperators["$mul"] = applyMul
	FieldUpdateOperators["$max"] = applyMax
	FieldUpdateOperators["$min"] = applyMin
	FieldUpdateOperators["$currentDate"] = applyCurrentDate
	FieldUpdateOperators["$push"] = applyPush
	FieldUpdateOperators["$pop"] = applyPop
	FieldUpdateOperators["$pull"] = applyPull
	FieldUpdateOperators["$pullAll"] = applyPullAll
	FieldUpdateOperators["$addToSet"] = applyAddToSet
	FieldUpdateOperators["$bit"] = applyBit
}

// Changes record the applied changes to a document.
type Changes struct {
	// Whether the operation was an upsert.
	Upsert bool

	// The fields that have been added, updated or removed in the document.
	// Added and updated fields are set to the final value while removed fields
	// are set to bsonkit.Missing.
	Changed map[string]interface{}

	// the temporary tree to track key conflicts
	pathTree bsonkit.PathNode
}

// Record will record a field change. If the value is bsonkit.Missing it will
// record a removal. It will return an error if a path is conflicting with a
// previous recorded change.
func (c *Changes) Record(path string, val interface{}) error {
	// check if path conflicts with another recorded change
	node, rest := c.pathTree.Lookup(path)
	if node.Load() == true || rest == bsonkit.PathEnd {
		return fmt.Errorf("conflicting key %q", path)
	}

	// add path to tree
	c.pathTree.Append(path).Store(true)

	// add change
	c.Changed[path] = val

	return nil
}

// Apply will apply a MongoDB update document on a document using the various
// update operators. The document is updated in place. The changes to the
// document are recorded and returned.
func Apply(doc, query, update bsonkit.Doc, upsert bool, arrayFilters bsonkit.List) (*Changes, error) {
	// check update
	if len(*update) == 0 {
		return nil, fmt.Errorf("empty update document")
	}

	// prepare changes
	changes := &Changes{
		Upsert:   upsert,
		Changed:  map[string]interface{}{},
		pathTree: bsonkit.NewPathNode(),
	}

	// update document according to update
	err := Process(Context{
		Value:                changes,
		TopLevel:             FieldUpdateOperators,
		MultiTopLevel:        true,
		TopLevelArrayFilters: arrayFilters,
		TopLevelQuery:        query,
	}, doc, *update, "", true)
	if err != nil {
		return nil, err
	}

	// recycle tree
	changes.pathTree.Recycle()
	changes.pathTree = nil

	return changes, nil
}

func applySet(ctx Context, doc bsonkit.Doc, _, path string, v interface{}) error {
	// set new value
	_, err := bsonkit.Put(doc, path, v, false)
	if err != nil {
		return err
	}

	// record change
	err = ctx.Value.(*Changes).Record(path, v)
	if err != nil {
		return err
	}

	return nil
}

func applySetOnInsert(ctx Context, doc bsonkit.Doc, _, path string, v interface{}) error {
	// check if upsert
	if !ctx.Value.(*Changes).Upsert {
		return nil
	}

	// set new value
	_, err := bsonkit.Put(doc, path, v, false)
	if err != nil {
		return err
	}

	// record change
	err = ctx.Value.(*Changes).Record(path, v)
	if err != nil {
		return err
	}

	return nil
}

func applyUnset(ctx Context, doc bsonkit.Doc, _, path string, _ interface{}) error {
	// remove value
	res := bsonkit.Unset(doc, path)
	if res == bsonkit.Missing {
		return nil
	}

	// record change
	err := ctx.Value.(*Changes).Record(path, bsonkit.Missing)
	if err != nil {
		return err
	}

	return nil
}

func applyRename(ctx Context, doc bsonkit.Doc, name, path string, v interface{}) error {
	// get new path
	newPath, ok := v.(string)
	if !ok {
		return fmt.Errorf("%s: expected string", name)
	}

	// TODO: We probably need to check whether indexes in the path are actually
	//  arrays. They might also reference an object field.

	// check path
	if bsonkit.IndexedPath(path) || bsonkit.IndexedPath(newPath) {
		return fmt.Errorf("%s: path cannot be an array", name)
	}

	// reject renames where source and target are identical
	if path == newPath {
		return fmt.Errorf("%s: source and target must differ", name)
	}

	// unset old value
	value := bsonkit.Unset(doc, path)
	if value == bsonkit.Missing {
		return nil
	}

	// set new value
	_, err := bsonkit.Put(doc, newPath, value, false)
	if err != nil {
		return err
	}

	// record remove
	err = ctx.Value.(*Changes).Record(path, bsonkit.Missing)
	if err != nil {
		return err
	}

	// record update
	err = ctx.Value.(*Changes).Record(newPath, value)
	if err != nil {
		return err
	}

	return nil
}

func applyInc(ctx Context, doc bsonkit.Doc, _, path string, v interface{}) error {
	// increment value
	res, err := bsonkit.Increment(doc, path, v)
	if err != nil {
		return err
	}

	// record change
	err = ctx.Value.(*Changes).Record(path, res)
	if err != nil {
		return err
	}

	return nil
}

func applyMul(ctx Context, doc bsonkit.Doc, _, path string, v interface{}) error {
	// multiply value
	res, err := bsonkit.Multiply(doc, path, v)
	if err != nil {
		return err
	}

	// record change
	err = ctx.Value.(*Changes).Record(path, res)
	if err != nil {
		return err
	}

	return nil
}

func applyMax(ctx Context, doc bsonkit.Doc, _, path string, v interface{}) error {
	// get value
	value := bsonkit.Get(doc, path)
	if value == bsonkit.Missing {
		// set value
		_, err := bsonkit.Put(doc, path, v, false)
		if err != nil {
			return err
		}

		// record change
		err = ctx.Value.(*Changes).Record(path, v)
		if err != nil {
			return err
		}

		return nil
	}

	// replace value if smaller
	if bsonkit.Compare(value, v) < 0 {
		// replace value
		_, err := bsonkit.Put(doc, path, v, false)
		if err != nil {
			return err
		}

		// record change
		err = ctx.Value.(*Changes).Record(path, v)
		if err != nil {
			return err
		}
	}

	return nil
}

func applyMin(ctx Context, doc bsonkit.Doc, _, path string, v interface{}) error {
	// get value
	value := bsonkit.Get(doc, path)
	if value == bsonkit.Missing {
		// set value
		_, err := bsonkit.Put(doc, path, v, false)
		if err != nil {
			return err
		}

		// record change
		err = ctx.Value.(*Changes).Record(path, v)
		if err != nil {
			return err
		}

		return nil
	}

	// replace value if bigger
	if bsonkit.Compare(value, v) > 0 {
		// replace value
		_, err := bsonkit.Put(doc, path, v, false)
		if err != nil {
			return err
		}

		// record change
		err = ctx.Value.(*Changes).Record(path, v)
		if err != nil {
			return err
		}
	}

	return nil
}

func applyCurrentDate(ctx Context, doc bsonkit.Doc, name, path string, v interface{}) error {
	// check if boolean
	value, ok := v.(bool)
	if ok {
		// set to time if true
		if value {
			// get time
			now := primitive.NewDateTimeFromTime(time.Now().UTC())

			// set time
			_, err := bsonkit.Put(doc, path, now, false)
			if err != nil {
				return err
			}

			// record change
			err = ctx.Value.(*Changes).Record(path, now)
			if err != nil {
				return err
			}
		}

		return nil
	}

	// coerce document
	args, ok := v.(bson.D)
	if !ok {
		return fmt.Errorf("%s: expected boolean or document", name)
	}

	// check document
	if len(args) != 1 || args[0].Key != "$type" {
		return fmt.Errorf("%s: expected document with a single $type field", name)
	}

	// get value
	var now interface{}
	switch args[0].Value {
	case "date":
		now = primitive.NewDateTimeFromTime(time.Now().UTC())
	case "timestamp":
		now = bsonkit.Now()
	default:
		return fmt.Errorf("%s: expected $type 'date' or 'timestamp'", name)
	}

	// set value
	_, err := bsonkit.Put(doc, path, now, false)
	if err != nil {
		return err
	}

	// record change
	err = ctx.Value.(*Changes).Record(path, now)
	if err != nil {
		return err
	}

	return nil
}

func applyPush(ctx Context, doc bsonkit.Doc, _, path string, v interface{}) error {
	// TODO: Add support for the modifiers {$each, $slice, $sort, $position}

	// push value
	res, err := bsonkit.Push(doc, path, v)
	if err != nil {
		return err
	}

	// record change if result is an array
	if array, ok := res.(bson.A); ok {
		addedPath := path + "." + strconv.Itoa(len(array)-1)
		err = ctx.Value.(*Changes).Record(addedPath, v)
		if err != nil {
			return err
		}
	}

	return nil
}

func applyPop(ctx Context, doc bsonkit.Doc, name, path string, v interface{}) error {
	// check value
	last := false
	if bsonkit.Compare(v, int64(1)) == 0 {
		last = true
	} else if bsonkit.Compare(v, int64(-1)) != 0 {
		return fmt.Errorf("%s: expected 1 or -1", name)
	}

	// pop element
	res, err := bsonkit.Pop(doc, path, last)
	if err != nil {
		return err
	}

	// check result
	if res == bsonkit.Missing {
		return nil
	}

	// record change
	err = ctx.Value.(*Changes).Record(path, bsonkit.Get(doc, path))
	if err != nil {
		return err
	}

	return nil
}

// pullMatches reports whether element matches the $pull condition. The
// condition can be a query expression (a document where every top-level key
// is an operator like $gte), a query against an embedded subdocument, or a
// scalar value compared by equality.
func pullMatches(element, condition interface{}) (bool, error) {
	if cd, ok := condition.(bson.D); ok {
		// distinguish element-level expression (all $-prefixed keys) from a
		// query against the element as a subdocument
		allOps := len(cd) > 0
		for _, e := range cd {
			if len(e.Key) == 0 || e.Key[0] != '$' {
				allOps = false
				break
			}
		}
		if allOps {
			virtual := bson.D{{Key: "_x", Value: element}}
			query := bson.D{{Key: "_x", Value: cd}}
			return Match(&virtual, &query)
		}

		// non-document elements never match a subdocument query
		ed, ok := element.(bson.D)
		if !ok {
			return false, nil
		}
		return Match(&ed, &cd)
	}

	// equality match
	return bsonkit.Compare(element, condition) == 0, nil
}

func applyPull(ctx Context, doc bsonkit.Doc, name, path string, v interface{}) error {
	// get target field
	field := bsonkit.Get(doc, path)
	if field == bsonkit.Missing {
		return nil
	}
	arr, ok := field.(bson.A)
	if !ok {
		return fmt.Errorf("%s: target field must be an array", name)
	}

	// build new array, dropping matching elements
	result := make(bson.A, 0, len(arr))
	removed := false
	for _, item := range arr {
		match, err := pullMatches(item, v)
		if err != nil {
			return err
		}
		if match {
			removed = true
			continue
		}
		result = append(result, item)
	}

	// no-op if nothing was removed
	if !removed {
		return nil
	}

	// store new array
	_, err := bsonkit.Put(doc, path, result, false)
	if err != nil {
		return err
	}

	// record change
	return ctx.Value.(*Changes).Record(path, result)
}

func applyPullAll(ctx Context, doc bsonkit.Doc, name, path string, v interface{}) error {
	// expect an array of values to remove
	targets, ok := v.(bson.A)
	if !ok {
		return fmt.Errorf("%s: expected array", name)
	}

	// get target field
	field := bsonkit.Get(doc, path)
	if field == bsonkit.Missing {
		return nil
	}
	arr, ok := field.(bson.A)
	if !ok {
		return fmt.Errorf("%s: target field must be an array", name)
	}

	// build new array, dropping any element equal to one of the targets
	result := make(bson.A, 0, len(arr))
	removed := false
	for _, item := range arr {
		match := false
		for _, target := range targets {
			if bsonkit.Compare(item, target) == 0 {
				match = true
				break
			}
		}
		if match {
			removed = true
			continue
		}
		result = append(result, item)
	}

	// no-op if nothing was removed
	if !removed {
		return nil
	}

	// store new array
	_, err := bsonkit.Put(doc, path, result, false)
	if err != nil {
		return err
	}

	// record change
	return ctx.Value.(*Changes).Record(path, result)
}

func applyAddToSet(ctx Context, doc bsonkit.Doc, name, path string, v interface{}) error {
	// resolve values to add: $each unwraps to a list, anything else is a
	// single value (which itself may be a document or array — those are
	// treated as opaque values, just like $push)
	var values bson.A
	if vd, ok := v.(bson.D); ok && len(vd) == 1 && vd[0].Key == "$each" {
		arr, ok := vd[0].Value.(bson.A)
		if !ok {
			return fmt.Errorf("%s: $each requires an array", name)
		}
		values = arr
	} else {
		values = bson.A{v}
	}

	// get current array; missing field becomes a new empty array
	field := bsonkit.Get(doc, path)
	var arr bson.A
	if field == bsonkit.Missing {
		arr = bson.A{}
	} else {
		var ok bool
		arr, ok = field.(bson.A)
		if !ok {
			return fmt.Errorf("%s: target field must be an array", name)
		}
	}

	// append unique values
	changed := false
	for _, val := range values {
		found := false
		for _, existing := range arr {
			if bsonkit.Compare(existing, val) == 0 {
				found = true
				break
			}
		}
		if !found {
			arr = append(arr, val)
			changed = true
		}
	}

	// no-op if nothing was added
	if !changed {
		return nil
	}

	// store new array
	_, err := bsonkit.Put(doc, path, arr, false)
	if err != nil {
		return err
	}

	// record change
	return ctx.Value.(*Changes).Record(path, arr)
}

func applyBit(ctx Context, doc bsonkit.Doc, name, path string, v interface{}) error {
	// expect a document with a single op (and / or / xor)
	spec, ok := v.(bson.D)
	if !ok || len(spec) != 1 {
		return fmt.Errorf("%s: expected document with a single bitwise op", name)
	}
	op := spec[0]

	// extract operand as integer; track whether it's int64-wide
	var operand int64
	var operandIs64 bool
	switch n := op.Value.(type) {
	case int32:
		operand = int64(n)
	case int64:
		operand = n
		operandIs64 = true
	default:
		return fmt.Errorf("%s: operand must be integer", name)
	}

	// extract current field value as integer; missing is treated as 0 so the
	// op can produce a fresh value (matching MongoDB semantics)
	field := bsonkit.Get(doc, path)
	var current int64
	var fieldIs64 bool
	switch n := field.(type) {
	case int32:
		current = int64(n)
	case int64:
		current = n
		fieldIs64 = true
	case bsonkit.MissingType:
		// stays at zero
	default:
		return fmt.Errorf("%s: target field must be integer", name)
	}

	// compute result
	var result int64
	switch op.Key {
	case "and":
		result = current & operand
	case "or":
		result = current | operand
	case "xor":
		result = current ^ operand
	default:
		return fmt.Errorf("%s: unknown bitwise op %q", name, op.Key)
	}

	// preserve int32 width unless either side was int64
	var resultVal interface{}
	if fieldIs64 || operandIs64 {
		resultVal = result
	} else {
		resultVal = int32(result)
	}

	// no-op if value would not change
	if field != bsonkit.Missing && bsonkit.Compare(field, resultVal) == 0 {
		return nil
	}

	// store new value
	_, err := bsonkit.Put(doc, path, resultVal, false)
	if err != nil {
		return err
	}

	// record change
	return ctx.Value.(*Changes).Record(path, resultVal)
}
