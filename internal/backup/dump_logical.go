package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
)

func dumpLogical(ctx context.Context, databaseURL string, note func(string)) ([]byte, error) {
	var buf bytes.Buffer
	if err := dumpLogicalToFile(ctx, databaseURL, note, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func dumpLogicalToFile(ctx context.Context, databaseURL string, note func(string), w io.Writer) error {
	if note == nil {
		note = func(string) {}
	}
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("подключение: %w", err)
	}
	defer conn.Close(ctx)

	fmt.Fprintln(w, "-- NetLynx logical dump (без pg_dump; восстановление через psql)")
	fmt.Fprintln(w, "SET client_encoding = 'UTF8';")
	fmt.Fprintln(w, "SET standard_conforming_strings = on;")
	fmt.Fprintln(w, "SET check_function_bodies = false;")
	fmt.Fprintln(w, "SET session_replication_role = replica;")
	fmt.Fprintln(w)

	seqs, err := listSequences(ctx, conn)
	if err != nil {
		return err
	}
	for _, s := range seqs {
		fmt.Fprintf(w, "CREATE SEQUENCE IF NOT EXISTS %s", pgQualify(s.schema, s.name))
		fmt.Fprintf(w, " START WITH %d INCREMENT BY %d MINVALUE %d MAXVALUE %d CACHE %d",
			s.start, s.increment, s.min, s.max, s.cache)
		if s.cycle {
			fmt.Fprint(w, " CYCLE")
		}
		fmt.Fprintln(w, ";")
	}
	if len(seqs) > 0 {
		fmt.Fprintln(w)
	}

	tables, err := listTables(ctx, conn)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		return fmt.Errorf("в БД нет таблиц")
	}

	for _, t := range tables {
		ddl, err := createTableSQL(ctx, conn, t)
		if err != nil {
			return fmt.Errorf("%s: %w", pgQualify(t.schema, t.name), err)
		}
		fmt.Fprintln(w, ddl)
		fmt.Fprintln(w)
	}

	owned, err := sequenceOwners(ctx, conn)
	if err != nil {
		return err
	}
	for _, o := range owned {
		fmt.Fprintf(w, "ALTER SEQUENCE %s OWNED BY %s.%s;\n",
			pgQualify(o.seqSchema, o.seqName), pgQualify(o.tblSchema, o.tblName), pgQuoteIdent(o.col))
	}
	if len(owned) > 0 {
		fmt.Fprintln(w)
	}

	var tail bytes.Buffer
	for _, t := range tables {
		note("логическая выгрузка: " + t.schema + "." + t.name)
		fmt.Fprintf(w, "COPY %s FROM stdin;\n", pgQualify(t.schema, t.name))
		sql := "COPY " + pgQualify(t.schema, t.name) + " TO STDOUT"
		tail.Reset()
		if _, err := conn.PgConn().CopyTo(ctx, &tail, sql); err != nil {
			return fmt.Errorf("COPY %s: %w", pgQualify(t.schema, t.name), err)
		}
		if _, err := io.Copy(w, &tail); err != nil {
			return err
		}
		if tail.Len() > 0 && tail.Bytes()[tail.Len()-1] != '\n' {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, `\.`)
		fmt.Fprintln(w)
	}

	for _, t := range tables {
		cons, err := constraintSQLs(ctx, conn, t)
		if err != nil {
			return fmt.Errorf("constraints %s: %w", pgQualify(t.schema, t.name), err)
		}
		for _, c := range cons {
			fmt.Fprintln(w, c)
		}
		idxs, err := extraIndexSQLs(ctx, conn, t)
		if err != nil {
			return fmt.Errorf("indexes %s: %w", pgQualify(t.schema, t.name), err)
		}
		for _, c := range idxs {
			fmt.Fprintln(w, c)
		}
	}
	if len(tables) > 0 {
		fmt.Fprintln(w)
	}

	for _, s := range seqs {
		var last int64
		var called bool
		q := "SELECT last_value, is_called FROM " + pgQualify(s.schema, s.name)
		if err := conn.QueryRow(ctx, q).Scan(&last, &called); err != nil {
			return fmt.Errorf("sequence %s: %w", pgQualify(s.schema, s.name), err)
		}
		fmt.Fprintf(w, "SELECT pg_catalog.setval(%s, %d, %t);\n",
			pgQuoteLiteral(s.schema+"."+s.name), last, called)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "SET session_replication_role = DEFAULT;")
	return nil
}

type relName struct {
	schema string
	name   string
	oid    uint32
}

type seqInfo struct {
	schema, name                      string
	start, increment, min, max, cache int64
	cycle                             bool
}

type seqOwner struct {
	seqSchema, seqName string
	tblSchema, tblName string
	col                string
}

func listSequences(ctx context.Context, conn *pgx.Conn) ([]seqInfo, error) {
	rows, err := conn.Query(ctx, `
		SELECT n.nspname, c.relname,
		       s.seqstart, s.seqincrement, s.seqmin, s.seqmax, s.seqcache, s.seqcycle
		FROM pg_sequence s
		JOIN pg_class c ON c.oid = s.seqrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		ORDER BY n.nspname, c.relname`)
	if err != nil {
		return nil, fmt.Errorf("список sequences: %w", err)
	}
	defer rows.Close()
	var out []seqInfo
	for rows.Next() {
		var s seqInfo
		if err := rows.Scan(&s.schema, &s.name, &s.start, &s.increment, &s.min, &s.max, &s.cache, &s.cycle); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func listTables(ctx context.Context, conn *pgx.Conn) ([]relName, error) {
	rows, err := conn.Query(ctx, `
		SELECT n.nspname, c.relname, c.oid
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind = 'r'
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		ORDER BY n.nspname, c.relname`)
	if err != nil {
		return nil, fmt.Errorf("список таблиц: %w", err)
	}
	defer rows.Close()
	var out []relName
	for rows.Next() {
		var t relName
		if err := rows.Scan(&t.schema, &t.name, &t.oid); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func createTableSQL(ctx context.Context, conn *pgx.Conn, t relName) (string, error) {
	rows, err := conn.Query(ctx, `
		SELECT a.attname,
		       pg_catalog.format_type(a.atttypid, a.atttypmod),
		       a.attnotnull,
		       a.atthasdef,
		       pg_catalog.pg_get_expr(ad.adbin, ad.adrelid),
		       a.attidentity,
		       a.attgenerated
		FROM pg_attribute a
		LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
		WHERE a.attrelid = $1
		  AND a.attnum > 0
		  AND NOT a.attisdropped
		ORDER BY a.attnum`, t.oid)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var name, typ, identity, generated string
		var notNull, hasDef bool
		var def *string
		if err := rows.Scan(&name, &typ, &notNull, &hasDef, &def, &identity, &generated); err != nil {
			return "", err
		}
		var b strings.Builder
		fmt.Fprintf(&b, "    %s %s", pgQuoteIdent(name), typ)
		switch strings.TrimSpace(identity) {
		case "a":
			b.WriteString(" GENERATED ALWAYS AS IDENTITY")
		case "d":
			b.WriteString(" GENERATED BY DEFAULT AS IDENTITY")
		default:
			if strings.TrimSpace(generated) == "s" && def != nil {
				fmt.Fprintf(&b, " GENERATED ALWAYS AS (%s) STORED", *def)
			} else if hasDef && def != nil && *def != "" {
				fmt.Fprintf(&b, " DEFAULT %s", *def)
			}
		}
		if notNull {
			b.WriteString(" NOT NULL")
		}
		cols = append(cols, b.String())
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(cols) == 0 {
		return "", fmt.Errorf("нет колонок")
	}
	return "CREATE TABLE IF NOT EXISTS " + pgQualify(t.schema, t.name) + " (\n" + strings.Join(cols, ",\n") + "\n);", nil
}

func sequenceOwners(ctx context.Context, conn *pgx.Conn) ([]seqOwner, error) {
	rows, err := conn.Query(ctx, `
		SELECT ns.nspname, s.relname, nt.nspname, t.relname, a.attname
		FROM pg_depend d
		JOIN pg_class s ON s.oid = d.objid AND s.relkind = 'S'
		JOIN pg_namespace ns ON ns.oid = s.relnamespace
		JOIN pg_class t ON t.oid = d.refobjid
		JOIN pg_namespace nt ON nt.oid = t.relnamespace
		JOIN pg_attribute a ON a.attrelid = d.refobjid AND a.attnum = d.refobjsubid
		WHERE d.deptype = 'a'
		  AND ns.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		ORDER BY ns.nspname, s.relname`)
	if err != nil {
		return nil, fmt.Errorf("OWNED BY: %w", err)
	}
	defer rows.Close()
	var out []seqOwner
	for rows.Next() {
		var o seqOwner
		if err := rows.Scan(&o.seqSchema, &o.seqName, &o.tblSchema, &o.tblName, &o.col); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func constraintSQLs(ctx context.Context, conn *pgx.Conn, t relName) ([]string, error) {
	rows, err := conn.Query(ctx, `
		SELECT conname, pg_catalog.pg_get_constraintdef(oid, true)
		FROM pg_constraint
		WHERE conrelid = $1 AND conparentid = 0
		ORDER BY CASE contype WHEN 'p' THEN 0 WHEN 'u' THEN 1 WHEN 'c' THEN 2 WHEN 'f' THEN 3 ELSE 4 END, conname`, t.oid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	qual := pgQualify(t.schema, t.name)
	for rows.Next() {
		var name, def string
		if err := rows.Scan(&name, &def); err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s %s;", qual, pgQuoteIdent(name), def))
	}
	return out, rows.Err()
}

func extraIndexSQLs(ctx context.Context, conn *pgx.Conn, t relName) ([]string, error) {
	rows, err := conn.Query(ctx, `
		SELECT pg_catalog.pg_get_indexdef(i.indexrelid)
		FROM pg_index i
		WHERE i.indrelid = $1
		  AND NOT i.indisprimary
		  AND NOT EXISTS (
		      SELECT 1 FROM pg_constraint c WHERE c.conindid = i.indexrelid
		  )
		ORDER BY i.indexrelid`, t.oid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var def string
		if err := rows.Scan(&def); err != nil {
			return nil, err
		}
		if def == "" {
			continue
		}
		if !strings.Contains(strings.ToUpper(def), " IF NOT EXISTS") {
			def = strings.Replace(def, "CREATE INDEX ", "CREATE INDEX IF NOT EXISTS ", 1)
			def = strings.Replace(def, "CREATE UNIQUE INDEX ", "CREATE UNIQUE INDEX IF NOT EXISTS ", 1)
		}
		if !strings.HasSuffix(def, ";") {
			def += ";"
		}
		out = append(out, def)
	}
	return out, rows.Err()
}

func pgQualify(schema, name string) string {
	return pgQuoteIdent(schema) + "." + pgQuoteIdent(name)
}

func pgQuoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func pgQuoteLiteral(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `''`) + `'`
}
