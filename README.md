# BIR Summary List of Sales and Purchases (.DAT) generator

A single-binary tool that turns a sales or purchases **CSV** into the BIR RELIEF **`.DAT`** file a VAT-registered taxpayer in the Philippines submits as the **Summary List of Sales and Purchases**.

It serves a small web interface on `127.0.0.1`, keeps its settings and recent files in one JSON file on the same machine, and makes no network calls of any kind. Your customers' and suppliers' names, TINs and figures never leave the computer.

## What it does

The whole product is getting the file right. A spreadsheet cannot produce it, because it is not a CSV export: the quoting rules differ between the header record and the detail records, nil is written one way in one and another way in the other, the header has to carry totals that match the records exactly, and the file name is read apart by the Bureau's validation module and checked against the contents.

So this tool:

- reads the CSV defensively — byte-order mark, any line ending, four separators, headings in the usual spellings, title rows above them, blank rows, short rows, thousands separators, a peso marker;
- adds together the rows naming the same taxpayer, because a summary list carries **one line per customer or supplier for the month**, and writes the records **in alphabetical order by name** as the regulation requires;
- builds the header and detail records from a **single field table**, checking as it goes that the writer and the table still agree;
- **validates every field** — width, required, characters, number format, date format, the totals — and **refuses to write the file at all** if anything would be rejected, naming each offending row and column by CSV line number;
- names the file the way the validation module expects;
- writes **both** the plain file and the copy carrying the submission validation key, in the single-byte encoding the format uses.

## Who it's for

VAT-registered small and medium businesses in the Philippines, and the bookkeepers and accountants who do their monthly and quarterly BIR work.

## The rule it implements

- **Revenue Regulations No. 16-2005, section 4.114-3** — a VAT-registered person must submit, with the quarterly return, a summary list of sales and a summary list of purchases, giving for each buyer and seller their name, TIN, address and the exempt, zero-rated and taxable amounts with the tax; **alphabetically arranged**, with the month's transactions **consolidated per customer or supplier**.
- The same section requires the list to conform to **the electronic format prescribed in a Revenue Memorandum Circular**, and permits three ways of producing it: the Bureau's Data Entry Module, an Excel template, or **the taxpayer's own extract program** — with the proviso that a taxpayer using their own program must run the result through **the Bureau's validation module**. This tool is that extract program. **It does not replace the validation module.**
- **Revenue Memorandum Circular No. 24-2002** prescribed the record format itself, as technical annexes.

Statutory provisions are cited as they stood when this was written; confirm the current position with the Bureau or a qualified practitioner.

## The record layout

Comma-separated, not fixed-width. Each record ends with **CR LF**. The file is **single-byte** (the Windows code page the Bureau's module was built on), not UTF-8. It holds **one header record, first, then its detail records** — there is **no trailer or control record**, because the header carries the totals, and no record count appears anywhere.

Two quoting conventions, which are not the same:

| | text field with a value | empty text field |
| --- | --- | --- |
| **header** | `"NAME"` | `""` |
| **detail** | `"NAME"` | nothing at all, so `,,` |

Two ways of writing an amount:

| | a figure | nil |
| --- | --- | --- |
| **header total** | `1234.56` | `0.00` |
| **detail amount** | `1234.56` | `0` |

There is no escape mechanism. A comma inside a value would split the record and the validation module refuses one; a quote would end the field early. Values are therefore cleaned before being written, and anything that cannot be cleaned is refused.

### Summary List of Sales — header, 17 fields

```
 1  H                        the record type
 2  S                        the transaction code
 3  TIN of owner             quoted, exactly 9 digits
 4  owner registered name    quoted, 50
 5  owner last name          quoted, 30
 6  owner first name         quoted, 30
 7  owner middle name        quoted, 30
 8  owner trade name         quoted, 50
 9  owner address line 1     quoted, 92   unit, street, barangay
10  owner address line 2     quoted, 74   district, city, postal code
11  total exempt sales       bare, always 2 decimal places
12  total zero-rated sales   bare
13  total taxable sales      bare
14  total output tax         bare
15  RDO code                 bare, exactly 3 characters — district 44 is 044
16  tax month                bare, MM/DD/YYYY, the LAST DAY of the month
17  accounting year-end      bare, 1 to 12, or empty
```

At least one of fields 4–8 must have a value, and at least one of 9–10.

### Summary List of Sales — detail, 15 fields, one per customer per month

```
 1  D                        the record type
 2  S                        the transaction code
 3  customer TIN             quoted when present, 9 digits; may be empty
 4  registered name          quoted, 50
 5  last name                quoted, 30
 6  first name               quoted, 30
 7  middle name              quoted, 30
 8  address line 1           quoted, 30
 9  address line 2           quoted, 30
10  exempt sales             bare, 2 decimal places, or 0 when nil
11  zero-rated sales         bare
12  taxable sales            bare, net of the tax
13  output tax               bare
14  TIN of owner             bare, NOT quoted — the filer's own 9 digits
15  tax month                bare, the same date as the header's
```

At least one of fields 4–7 must have a value. A company fills 4 and leaves 5, 6 and 7 empty; an individual does the opposite. A customer's TIN may be empty, for a sale to someone not registered.

### Summary List of Purchases

**Header, 21 fields:** the same first 10, then totals for exempt, zero-rated, taxable services, taxable capital goods, taxable other goods and input tax, then the **creditable** and **non-creditable** input tax, then the RDO code, tax month and accounting year-end. The validation module insists that the total input tax equals the creditable and non-creditable amounts added together.

**Detail, 17 fields:** the same first 9, then exempt purchases, zero-rated purchases, taxable purchases of services, of capital goods and of goods other than capital goods, the input tax, the filer's TIN and the tax month. Unlike a sales record, a purchases record **must** give the supplier's TIN and an address.

### The file name

```
<9-digit TIN><S or P><MM><YYYY>.DAT

004123456S102026.DAT    sales for October 2026
004123456P102026.DAT    purchases for the same month
```

The validation module reads this apart by position and checks it against the contents. **One file per month** — a VAT quarter is three of each kind.

### The TIN, and why there is no branch code

Every TIN field holds **exactly nine digits**: no dashes, and **no branch code anywhere** — not in the header, not in a detail record, not in the file name. A branch code belongs to other BIR formats. If your CSV carries one it is read, reported, and left out. A TIN that is a run of a single digit is refused, because the validation module refuses it.

### Characters

The validation module refuses the control characters, `" $ & ' , \ `` |` and everything above plain ASCII — with one deliberate exception, `Ñ`, which Filipino surnames need.

Two of those are common in business names, so they are substituted and the substitution is reported: an **ampersand becomes the word AND** (`SMITH & SONS` → `SMITH AND SONS`) and an **apostrophe is removed** (`O'BRIEN` → `OBRIEN`). Other accented letters are folded to their plain form, and text is upper-cased as the Bureau's own entry screens upper-case it. Nothing is altered silently.

### The submission copy

The Bureau's validation module writes a second copy of a validated file into a folder of its own with one extra line at the end: a twenty-character **validation key** derived from the file name, the number of records, their total length and a checksum of their contents — and tells the user that is the copy to attach for electronic submission.

This tool produces both. The plain file goes to the validation module; the copy in `eRelief/` carries the key.

## Install & run

One Go program, standard library only — nothing to install, nothing to configure.

```sh
go build -o bir-slsp-dat-generator-csv .
./bir-slsp-dat-generator-csv
```

Or without building:

```sh
go run .
```

Then open `http://127.0.0.1:8080/`. Two flags change the defaults:

```sh
./bir-slsp-dat-generator-csv -addr 127.0.0.1:9000 -data ~/bir/slsp-data.json
```

It also runs once from the command line, for a month-end script. It exits **0** when the file was written, **1** when it was refused, **2** when the arguments or the CSV could not be used at all:

```sh
./bir-slsp-dat-generator-csv \
    -csv sales-october.csv -list sales -period 10/2026 \
    -tin 004-123-456-000 -name "ABC Trading Corporation" \
    -address "123 Main St Brgy San Antonio|Makati City 1203" \
    -rdo 044 -out ./filings/

# print the whole record layout and stop
./bir-slsp-dat-generator-csv -layout

# write a CSV template to start from
./bir-slsp-dat-generator-csv -sample purchases > purchases-template.csv
```

For a purchases list, `-non-creditable` gives the part of the month's input tax that is not creditable; the rest is treated as creditable, and the two must add up to the total in the file.

### The CSV

Only the **TIN** column has to be present. Everything else is matched by heading — the usual spellings used by spreadsheets and accounting packages are understood — and a missing figure counts as nil. This works:

```csv
TIN,Type,Registered Name,Surname,First Name,Middle Name,Address 1,Address 2,Taxable Sales,Output Tax
123-456-789-000,N,Acme Trading Corporation,,,,12 Ayala Avenue,Makati City,100000.00,12000.00
345-678-901-000,I,,Santos,Maria Clara,Reyes,7 Rizal Street,Davao City,18000.00,2160.00
```

and produces this:

```
H,S,"004123456","ABC TRADING CORPORATION","","","","ABC TRADING","123 MAIN ST","MAKATI CITY",0.00,0.00,118000.00,14160.00,044,10/31/2026,12
D,S,"345678901",,"SANTOS","MARIA CLARA","REYES","7 RIZAL STREET","DAVAO CITY",0,0,18000.00,2160.00,004123456,10/31/2026
D,S,"123456789","ACME TRADING CORPORATION",,,,"12 AYALA AVENUE","MAKATI CITY",0,0,100000.00,12000.00,004123456,10/31/2026
```

A company is named by its **registered name**, an individual by **surname, first name and middle name** — one or the other on a record, never both. If your CSV has a single name column, write an individual as `SURNAME, First Middle`: the comma is how the tool knows where a surname like `DELA CRUZ` ends, and without it the row is refused rather than guessed at.

Amounts may carry a `₱`, `PHP` or `Php` marker and thousands separators. A cell that cannot be read is reported against its line and column, never treated as nil.

## Where data is stored

One JSON file on your own computer — `./bir-slsp-dat-generator-csv-data.json` unless `-data` says otherwise — holding the filing taxpayer's details so you do not retype them each month, and the last 25 files you built so you can download one again. It is written atomically (temporary file, then rename) so an interrupted write cannot corrupt it, and it is created readable only by you. Delete that file and the tool knows nothing.

The tool binds to `127.0.0.1`, makes no outbound network calls, has no accounts, analytics or telemetry, and loads no fonts, scripts or styles from anywhere but itself.

## Assumptions

- **Value-added tax at 12%** — the rate imposed under the National Internal Revenue Code as amended. It is a single named constant (`slsp.VATRatePercent`), used only to *check* that a tax column is consistent with its taxable column, never to work the tax out. A figure more than ₱1.00 from the rate is remarked on, never refused, because a party may be partly zero-rated or exempt. `-vat-rate` overrides it for a period when another rate applied. Confirm the current rate with the Bureau.
- **Header totals must match the detail records.** The validation module allows up to ₱2.00 of difference; this tool makes them agree exactly.
- **Amounts are whole centavos**, so a thousand records total exactly with no drift.
- **Negative amounts are refused** by default: the list has no column for one. Net returns, discounts and credit memos against the month's figures, or pass `-allow-negative`.
- **One month per file.** The tax month is the last day of the month, on every record, leap years included.
- **Field widths** are the Bureau's module's own: 50 for a registered or trade name, 30 for each part of an individual's name and each of a party's two address lines, 92 and 74 for the filer's two, 3 for the RDO code. A longer value is shortened and you are told.
- **Ceilings the validation module imposes:** at most 25 fields per record and 254 characters per field; and at most 600 characters per record, because the routine that appends the submission key reads no further and drops the rest without a word. A record that long is refused.

## Scope & accuracy

The layout implemented here was taken from the Bureau's own RELIEF data entry module and its matching validation module — from the routine that writes the file, cross-checked field by field against the routine that accepts or rejects it. Both agree. `-layout` prints every field together with how well it could be sourced.

What could **not** be established, and what it means:

- **The prescribing annexes to RMC 24-2002** could not be obtained in published form. The layout comes from the Bureau's reference implementation instead — arguably the stronger source, since the validation module is what accepts or rejects your file, but it is not the text of the circular.
- **Whether the module we read is the current one** could not be confirmed. It is the latest we could obtain and no later version is referred to anywhere, but the Bureau's download pages have since moved. **Check the version offered on the Bureau's own site before relying on this for a filing.**
- **No real, published `.DAT` file** could be found to compare against byte for byte. The output was derived from the writing routine and checked against the validating routine, not against a specimen.
- **Whether electronic submission still requires the validation key line** could not be confirmed. The algorithm is certain; its current necessity is not. Both copies are produced.
- **CR LF line endings** follow from the file-writing call the module uses, not from a published specification.
- **The regulation asks for more than the format carries.** RR 16-2005 lists "sales subject to final VAT withheld" and the purchases equivalent as required content, but the RELIEF record layout has no field for them. This tool follows the layout, because that is what the validation module checks.
- Only the **sales** and **purchases** lists are produced. The importations list, which the same module also writes, is not.

This tool builds a file from figures you give it. It does not check them against your books, your returns or your invoices, it does not decide what is taxable, exempt or zero-rated, and it is not tax, accounting or legal advice. **Run the file through the Bureau's own validation module before you submit it**, and confirm the current format, the deadlines and the submission channel with the Bureau of Internal Revenue or a qualified practitioner.

## Tests

The rules live in `internal/slsp` as pure functions with table-driven tests: the field widths and formats, the TIN shape, the date format and the last-day-of-month arithmetic through leap years, amount parsing and formatting, the character set, per-row validation, the consolidation and ordering, the submission key, and a **golden-file test asserting the exact bytes** of a small generated `.DAT` — checked both against committed fixtures and, separately, against the format's own rules, so the fixtures cannot merely prove the program consistent with itself.

```sh
gofmt -l .
go vet ./...
go test ./... -count=1
go build -o bir-slsp-dat-generator-csv .
```

Run the golden test with `UPDATE_GOLDEN=1` after a deliberate change to the layout, then read the diff before committing it.

MIT licensed.
