package knowledge

// noteSearchBody is the note text plus each own variable's name and computed value.
// The formula source stays out. Secrets live in another table and are not here.
// The value column is rewritten when the variable is saved, so the search reads the current text.
const noteSearchBody = `COALESCE(n.description,'') || ' ' || COALESCE((SELECT string_agg(nv.name || ' ' || nv.value, ' ') FROM named_value nv WHERE nv.note_id = n.id), '')`

// noteVarPredicate matches one bound spec: `name` or `name=value`.
// One placeholder, read through a scalar, so the name and the value are not interpolated.
const noteVarPredicate = `EXISTS (
  SELECT 1 FROM (SELECT ?::text AS spec) s
  JOIN named_value nv ON nv.note_id = n.id
  WHERE split_part(s.spec, '=', 1) <> ''
    AND nv.name = split_part(s.spec, '=', 1)
    AND (strpos(s.spec, '=') = 0 OR nv.value = substr(s.spec, strpos(s.spec, '=') + 1))
)`
