package parser

import "testing"

func TestParseCreateTableColumnsParsesSimpleColumns(t *testing.T) {
	columns, err := ParseCreateTableColumns(`CREATE TABLE smoke.smoke_table (
		id INT NOT NULL CONSTRAINT PK_smoke_table PRIMARY KEY,
		value NVARCHAR(100) NULL
	);`)
	if err != nil {
		t.Fatal(err)
	}
	if len(columns) != 2 {
		t.Fatalf("expected two columns, got %#v", columns)
	}
	if columns[0].NormalizedName != "id" || columns[0].TypeName != "int" || columns[0].Nullable {
		t.Fatalf("unexpected first column: %#v", columns[0])
	}
	if columns[1].NormalizedName != "value" || columns[1].TypeName != "nvarchar" || columns[1].Length != 100 || !columns[1].Nullable || !columns[1].AutoAddEligible {
		t.Fatalf("unexpected second column: %#v", columns[1])
	}
}

func TestParseCreateTableColumnsRejectsMultipleBatches(t *testing.T) {
	if _, err := ParseCreateTableColumns("CREATE TABLE smoke.t(id int);\nGO\nSELECT 1;"); err == nil {
		t.Fatal("expected multiple batch rejection")
	}
}

func TestParseCreateTableColumnsMarksNotNullAdditionUnsupported(t *testing.T) {
	columns, err := ParseCreateTableColumns(`CREATE TABLE smoke.smoke_table (
		id INT NOT NULL,
		value NVARCHAR(100) NOT NULL
	);`)
	if err != nil {
		t.Fatal(err)
	}
	if columns[1].AutoAddEligible {
		t.Fatalf("expected NOT NULL addition to stay unsupported, got %#v", columns[1])
	}
}
