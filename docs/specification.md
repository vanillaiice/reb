# Rebar `.reb` Template Format: Technical Specification & Compiler Rules

This document provides a formal technical specification of the Rebar (`.reb`) template format. It is designed to act as a definitive reference for engineers authoring parsers, compiling services, and AI systems generating valid Rebar templates.

---

## 1. Architectural Design Goal
The `.reb` (Rebar Template Layout) format is a hybrid declarative markup structure. Its primary goal is to **unify input schemas and presentation layouts in a single file**, eliminating synchronization drift between web database schemas and printed PDF documents. 

A `.reb` template is compiled by the Go backend (and frontend editor) into:
1. **JSON Schema Array**: Extracted from custom `<reb-*>` elements and used by the web client to render active forms dynamically.
2. **Go Template HTML Structure**: Custom tags are transpiled into standard HTML with `{{.name}}` data bindings (answers sit at the root of the template data, section 5) used by the headless Chromium compiler (Gotenberg) to output high-fidelity vector PDFs.

---

## 2. File Topology
A standard `.reb` file uses standard HTML5 markup combined with custom Rebar form elements.

```mermaid
graph TD
    A[Rebar .reb File] --> B[Standard HTML Structure]
    A --> C[Presentation Style: &lt;style&gt;]
    A --> D[Custom Form Elements: &lt;reb-*&gt;]
    D --> E[Compiled to JSON Schema Array]
    D --> F[Transpiled to Go {{.name}} Bindings]
```

### Topology Details:
1. **Custom `<reb-*>` Elements**: XML-style tags used to declare interactive form inputs.
2. **`<style>` Block**: Standard CSS stylesheet overrides. Rules declared here apply globally during HTML-to-PDF compilation.
3. **HTML Presentation Body**: Pure HTML5 markup providing the structural layout.

---

## 3. Custom Form Elements (`<reb-*>`)
Instead of a separate metadata block, `.reb` files use inline custom tags to declare form fields. The compiler reads these tags to generate the `fields_schema` and then replaces them with their respective Go template binding representation.

### Supported Elements:
* `<reb-declare>`: **Invisible element.** Declares a field for the web form but deletes itself from the final HTML output. Use this when you want to place raw data bindings like `{{.subcontractor}}` anywhere in your layout. **Note:** You can group inputs visually in the form builder by adding `<reb-declare type="section" name="unique_id" label="Group Title" />`. This creates a section divider in the UI without affecting the final PDF layout.
* `<reb-tailwind>`: **Configuration element.** Injects the Tailwind CSS compiler into the PDF renderer, allowing you to use native Tailwind utility classes.
* `<reb-pagebreak>`: **Layout element.** Forces a hard page break in the output PDF document (`page-break-after: always`).
* `<reb-footer>`: **Layout element.** Defines a repeating footer for the PDF document. Use `<span class="pageNumber"></span>` and `<span class="totalPages"></span>` inside it for automatic Gotenberg pagination!
* `<reb-header>`: **Layout element (v1.1).** The counterpart of `<reb-footer>`, repeated at the top of every page, with the same page-number spans.
* `<reb-checkbox>`: **(v1.1)** A tick box; the answer is `true` or `false`, so test it with `{{if .name}}`.
* `<reb-radio>`: Choices shown as radio buttons; `options` as for `<reb-select>`.
* `<reb-attachments>`: Like `<reb-photogrid>` (a list of images).
* `<reb-text>`: Standard single-line text input.
* `<reb-number>`: Numeric parameters.
* `<reb-textarea>`: Multi-line text block.
* `<reb-date>`: Interactive calendar date-picker.
* `<reb-select>`: Dropdown selection box.
* `<reb-photogrid>`: Renders an upload zone for multiple photos.
* `<reb-signature>`: Digital signature pad; the signature is stored as a PNG image.

### 3.1 Element Attributes
Every `<reb-*>` element supports the following attributes:
* **`name`** (`string`, Required): The unique alphanumeric identifier for the field. Used as the binding key in the JSON schema and Go template. Must match regex `^[A-Za-z_][A-Za-z0-9_]*$` (letters, digits and underscores, not starting with a digit); the compiler rejects anything else, because the name becomes a `{{.name}}` template binding.
* **`label`** (`string`, Required): The human-readable label rendered next to the input field in the web client.
* **`options`** (`comma-separated string`, Optional): Mandated only when using `<reb-select>`. Represents allowed selection values (e.g., `options="High,Medium,Low"`).
* **`class`** (`string`, Optional): Standard CSS/Tailwind classes to apply to the output element during PDF generation.

### 3.2 Form behaviour attributes (v1.1)
These change how the form asks for the answer; they do not change the PDF layout. Every consumer
(Rebar's web form and server, Rebar Studio) enforces them the same way, through the engine.

* **`required`**: the field must be answered (a checkbox must be ticked, a photo grid or table must
  have at least one entry). Only checked while the field is shown (see `show-if`).
* **`help`**: a hint shown under the input.
* **`placeholder`**: text shown in an empty text or number input.
* **`default`**: the value a new document starts with. `default="today"` on a date is the day the
  document is created; `default="true"` ticks a checkbox.
* **`min`**, **`max`**: bounds for a number, or for a date written `YYYY-MM-DD`.
* **`step`**: the increment a number input offers (a hint, not checked).
* **`pattern`**: a regular expression the whole text answer must match (as HTML's `pattern`).
* **`show-if`**: the field is shown only while this condition holds (3.3). A hidden field's answer
  is not kept.
* **`fillable`**: on `<reb-text>`, `<reb-number>`, `<reb-date>`, `<reb-textarea>` and
  `<reb-checkbox>`, the answer can also be typed (or ticked) into the PDF (section 4.5). Elsewhere it has no effect and compiles with a
  `fillable_ignored` warning.

### 3.3 `show-if` conditions
A condition reads other fields' answers:

```
expression := or
or         := and ("or" and)*
and        := not ("and" not)*
not        := "not" not | comparison
comparison := operand (("==" | "!=") operand)?
operand    := field_name | 'text' | "text" | number | "(" expression ")"
```

* A field alone is true when it is answered: non-blank text other than `false`, a ticked
  checkbox, a non-empty list, a number other than 0.
* `==` and `!=` compare the answers as trimmed text: a checkbox reads `true` or `false`, a
  missing answer reads as empty.
* Conditions are evaluated repeatedly, so a field shown only by a hidden field is hidden as well.
* Examples: `show-if="work_type == 'Hot work'"`, `show-if="permit and not isolated"`,
  `show-if="(shift == 'Night' or crew != 0) and lighting"`.

A condition that does not parse is a compile error; a condition naming a field the template does not
declare is a warning. The cases in `testdata/show_if_cases.json` define the behaviour.

---

## 4. Compiler Transformations & Bindings
During compilation, the `rebcompiler` removes `<reb-*>` tags and replaces them with standard HTML wrappers containing data bindings. The bindings are injected directly at the root of the Go template context.

### 4.1 Layouts and Page Sizing
By default, the Rebar PDF generator enforces the global standard **A4 paper size** (8.27 × 11.69 inches). You can override this sizing and configure custom page margins globally using standard CSS `@page` rules inside your `<style>` block:
```css
@page {
  size: A4 portrait;
  margin: 1.5cm;
}
```

#### Mixing Portrait and Landscape
The underlying Chromium compiler fully supports named `@page` rules. This allows you to mix portrait and landscape pages within the same document!
```html
<style>
  /* Default page is portrait */
  @page { size: A4 portrait; margin: 1cm; }
  
  /* Define a named page rule for landscape */
  @page landscape { size: A4 landscape; margin: 1cm; }
  
  /* Assign the named rule to a specific container */
  .landscape-page { 
    page: landscape; 
    page-break-before: always; 
  }
</style>

<div>
  <h1>Portrait Page</h1>
  <p>Standard flow will be in portrait.</p>
</div>

<!-- This div and its contents will be forced onto a landscape page -->
<div class="landscape-page">
  <h1>Landscape Page</h1>
  <p>Perfect for wide tables or large diagrams.</p>
</div>
```

### 4.2 Standard Replacements
A standard tag like `<reb-text name="project_id" class="font-bold" />` will be transpiled into:
```html
<span class="font-bold">{{.project_id}}</span>
```

### 4.3 Complex Replacements
Elements like `<reb-photogrid>` or `<reb-textarea>` are wrapped in block-level `<div>` elements instead of `<span>`. 
For example, `<reb-photogrid name="site_photos" />` transpiles to iterate over the photo array:
```html
<div>
  {{range .site_photos}}
    <img src="{{.}}" class="w-full object-cover rounded shadow-sm" />
  {{end}}
</div>
```

#### `<reb-table>`
Declares a dynamic row-based table.

**Important**: The `options` attribute defines the schema columns for the *frontend data entry form*, formatted as `col1:type,col2:type`.

Supported column types:
- `text`: Standard text input (default if type is omitted)
- `number`: Numeric input
- `photo` / `image`: Upload button for inline table images. Supports drag-and-drop and Ctrl+V clipboard pasting.
- `signature`: Digital signature pad component for field authorizations.
- `checkbox`: Boolean checkbox
- `select[Option1|Option2|Option3]`: Dropdown select menu with options separated by `|`
- `formula[expression|precision]`: Read-only computed field evaluated in real-time. Example: `formula[qty*rate|2]`.
  The expression uses numbers, the row's column names, `+ - * /` (with the usual precedence, and `-`
  or `+` before a value, as in `qty*-rate`) and parentheses. A column reads as its number (0 when
  empty or not a number), any other name as 0; dividing by zero gives 0, and so does an expression
  that does not parse. The result has `precision` decimals (2 when omitted, at most 10). The cases
  in `testdata/formula_cases.json` define the behaviour.
- `autoincrement`: Read-only field that automatically renders the current row's numeric index (1, 2, 3...).

The backend PDF compiler allows you to design your table layout *manually*. You can write standard HTML `<table>` elements inside `<reb-table>`, and add the `reb-row` attribute to your row template `<tr>`. The compiler will automatically loop over the table data and repeat the `<tr>` block for each row entered by the user.

**Example**:
```html
<reb-table name="defects" label="Defects List" options="no:text,defect:text,severity:text,photo:photo">
  <table class="w-full text-left text-sm border-collapse border border-slate-300">
    <thead class="bg-slate-100">
      <tr>
        <th class="p-2 border border-slate-300">No</th>
        <th class="p-2 border border-slate-300">Defect</th>
        <th class="p-2 border border-slate-300">Severity</th>
        <th class="p-2 border border-slate-300">Photo</th>
      </tr>
    </thead>
    <tbody>
      <tr reb-row class="border-b border-slate-200">
        <td class="p-2 border-r border-slate-300">{{.no}}</td>
        <td class="p-2 border-r border-slate-300">{{.defect}}</td>
        <td class="p-2 border-r border-slate-300 font-bold">{{.severity}}</td>
        <td class="p-2 text-center">
          {{if .photo}}
            <img src="{{.photo}}" class="h-16 w-16 object-cover mx-auto rounded" />
          {{else}}
            <span class="text-xs text-slate-400">No Photo</span>
          {{end}}
        </td>
      </tr>
    </tbody>
  </table>
</reb-table>
```

### 4.4 Built-in Math Functions
The PDF compiler registers custom Go template functions for complex layout formulas. These functions safely parse and cast strings to floats, ensuring your calculations never panic.

- `{{multiply .a .b}}`
- `{{add .a .b}}`
- `{{subtract .a .b}}`
- `{{divide .numerator .denominator}}`
- `{{sumColumn .my_table "amount"}}`: Extracts all rows from a table array, plucks the target key, and calculates the total sum.
- `{{formatNumber .val 2}}`: Forces floating point numbers to render with the specified number of decimal places.
- `{{formatMoney .val "QAR" 2}}`: A number with thousands separators, the given decimals and the currency code (also `{{sumColumn .rows "total" | formatMoney "QAR" 2}}`).
- `{{formatDate "02/01/2006" .day}}`: A date answer in Go's layout notation; `{{now | formatDate "02/01/2006"}}` prints today.
- `{{safeHTML .value}}`: Prints a value as HTML (empty when missing). Answer text is sanitized first (bluemonday's UGC policy: formatting and links stay, scripts, styles and event handlers go); a text area's paragraphs print as they are.

**Example Grand Total:**
```html
<div class="text-right font-bold text-lg">
  Grand Total: ${{sumColumn .defects "amount" | formatNumber 2}}
</div>
```

### 4.5 Fillable PDF fields
**Experimental.** This section may change or be removed in a minor release (8.4); a consumer that
uses it pins the engine release.

A document can be rendered in one of two modes, chosen by the consumer each time it renders
(`"fillable"` in the render input, section 8):

* **with data** (the default): every field prints its answer, as above;
* **fillable**: a field marked `fillable` prints an empty box instead of its answer, and the PDF
  made from the page carries a text field over each box (a check box over a checkbox's), so someone
  without Rebar can fill the PDF in any PDF reader. Fields not marked `fillable` still print their answers, so a document can go out
  half filled.

In fillable mode, `<reb-text name="supplier" label="Supplier" fillable class="w-64" />` transpiles to
a link whose target names the field:
```html
<a href="reb-field:supplier" class="reb-fillable w-64"></a>
```
(`reb-field:remarks;multiline` and the extra class `reb-fillable-multiline` for a text area,
`reb-field:crane;checkbox` and `reb-fillable-checkbox` for a checkbox). The
compiled template holds both forms behind `{{if $.Fillable}}`. A built-in style, which any class
the template sets overrides, makes the box an underlined `10em` wide line (a text area: a framed
block the width of its container, `5em` high; a checkbox: a framed `1em` square); size it with classes like any other element. A field
printed twice gives two boxes of one PDF field, which share the value.

Templates can test the mode themselves: `{{if .Fillable}}Fill in the boxes{{end}}`.

Chromium flattens form inputs when it prints but keeps links, so the consumer turns the printed page
into a form afterwards: `rebc fillable` (or `__rebFillable`) replaces each `reb-field:` link with a
text field named after the field, pre-filled with the answers it is given, and each checkbox link with
a check box, ticked when its answer is. When the filled PDF comes back, `rebc pdf-answers` (or
`__rebPdfAnswers`) reads the values of the template's fillable fields: plain text (a checkbox: `true`
or `false`), to be checked with `prepare` and kept as the document's answers. Other fields the PDF
may have are ignored.

---

## 5. Rendering documents

### 5.1 System values
Besides its own fields, a template can print these values of the document. A consumer passes them
all (empty when it has none); an answer never overrides one.

| Value | Meaning |
|---|---|
| `{{.ID}}` | the document's identifier |
| `{{.Name}}` | the document's title |
| `{{.Number}}`, `{{.Reference}}` | its number in the project, and its reference (e.g. `D-12`) |
| `{{.ProjectName}}` | the project's name |
| `{{.ReporterName}}` | who created the document |
| `{{.TemplateName}}` | the template's name |
| `{{.CreatedAt}}` | when it was created (ISO 8601; format with `formatDate`) |
| `{{.OrganizationName}}` | the organization (or, in Rebar Studio, the profile) the document belongs to |
| `{{.OrganizationLogo}}` | its logo as an image source (`<img src="{{.OrganizationLogo}}">`), empty when there is none |
| `{{.Attachments}}`, `{{.Photos}}` | the document's files (each with `.FileName`, `.ObjectKey` as image source) |
| `{{.Answers}}` | all the answers, also available at the top level |
| `{{.Fillable}}` | true when the document is rendered fillable (section 4.5); set by the engine |

### 5.2 Template assets
A template may carry images (a logo, a stamp). It refers to them by bare file name,
`<img src="logo.png">` or `url(logo.png)` in CSS, never by URL: no renderer has network access. A
consumer delivers the files under those names (Rebar sends them to Gotenberg next to the page;
Rebar Studio packs them into `.rebpack` files under `assets/`). File names use letters, digits, `.`,
`-` and `_`.

### 5.3 Answers in the template
* File answers (photos, signatures, photo cells) arrive as the file names the renderer receives,
  usable in `src`.
* Text areas hold plain text. They render as paragraphs: a blank line starts a new paragraph, a line
  break becomes `<br>`, and the text is escaped.
* Other answers are plain text: the template escapes them where it prints them, so `<b>` typed in a
  text field prints as `<b>`, and comparisons such as `{{if eq .dept "R&D"}}` see what was typed.
  `{{safeHTML .name}}` prints an answer as sanitized HTML instead.
* Table rows are lists of objects keyed by column; formula and row-number cells are computed by the
  engine (`testdata/formula_cases.json`), whatever the form sent.

---

## 6. Tokenization & Parsing Strategy (For Parsers & AI)
When compiling a `.reb` template, compilers or AI parsers should follow this process:

### Phase 1: AST Extraction
Parse the document using an HTML/XML parser (like Go's `golang.org/x/net/html` or JS's `DOMParser`). Traverse the DOM tree looking for elements starting with `reb-`.

### Phase 2: Schema Generation
For every matched `<reb-*>` node, extract the tag type (omitting `reb-`), `name`, `label`, and `options`. Push these into a structured JSON array representing the fields schema.

### Phase 3: Transpilation
Mutate the `<reb-*>` nodes in the AST in-place. Change the node tag to `span` (or `div`), strip Rebar-specific attributes (preserving `class`), and inject the `{{.NAME}}` Go template syntax as text nodes. Serialize the AST back to a string. Note: `<reb-declare>` tags must be completely removed from the AST.

---

## 7. Complete Valid File Example

```html
<!-- REBAR COMPILER v1.0 SPECIFICATION STANDARD -->
<reb-tailwind></reb-tailwind>

<style>
  body {
    font-family: Arial, sans-serif;
    color: #1a202c;
    line-height: 1.6;
  }
  .header-card {
    border-left: 6px solid #d97706;
    background-color: #fef3c7;
    padding: 16px;
    margin-bottom: 20px;
  }
  .meta-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px;
    margin-bottom: 30px;
  }
</style>

<div class="header-card">
  <h2>High Risk Work Authorization Certificate</h2>
</div>

<div class="meta-grid">
  <div>
    <strong>Permit Reference:</strong>
    <reb-number name="permit_number" label="Authorized Permit Number" class="text-slate-800 font-semibold"></reb-number>
  </div>
  <div>
    <strong>Clearance Category:</strong>
    <reb-select name="clearance_type" label="Operational Clearance Category" options="Hot Work, Confined Space Entry, Electrical Isolation"></reb-select>
  </div>
</div>

<div class="status-box">
  <p><strong>PPE Controls Verified:</strong> 
    <reb-text name="ppe_verified" label="Protective Gears Inspected"></reb-text>
  </p>
  
  <h3 class="mt-4 font-bold">Additional Notes</h3>
  <reb-textarea name="notes" label="Site Supervisor Remarks"></reb-textarea>
</div>

<reb-footer>
  <div style="width: 100%; text-align: center; font-size: 10px; color: #718096; padding-top: 10px; border-top: 1px solid #cbd5e1; margin-top: 20px;">
    Rebar Automated Document System — Page <span class="pageNumber"></span> of <span class="totalPages"></span>
  </div>
</reb-footer>
```

---

## 8. Engine interface

The engine (this repository) is the only implementation of the rules above. Consumers call it as
`rebc` (JSON on stdin and stdout) or as the WebAssembly build (`__rebCompile`, `__rebPrepare`,
`__rebRender`, `__rebVersion`). The PDF form functions of section 4.5 (`rebc fillable` and
`pdf-answers`) are a second WebAssembly build, `rebpdf.wasm` (`__rebFillable`, `__rebPdfAnswers`),
loaded only when needed. PDFs travel base64-encoded in the JSON.

### 8.1 Schemas
`compile` returns two schemas. `schema` is the raw list the tags declared (`key`, `type`, `label`,
`options`, the v1.1 attributes and `fillable`; schemaVersion 1, read by older clients). `fields` is the normalized
schema, `{"schemaVersion": 2, "fields": [...]}`: each field has a `kind` (`section`, `text`,
`number`, `date`, `textarea`, `select`, `checkbox`, `images`, `signature`, `table`; aliases such as
`radio`, `string` and `photogrid` mapped, the declared `type` kept), a `maxLength` for text,
`fillable` when the field can be typed into the PDF (section 4.5), and
tables have parsed `columns` (`key`, `kind`, `label`, `options`, `expression`, `precision`) with the
columns declared inside them folded in. Consumers read `fields` and never parse options themselves.

### 8.2 Errors and warnings
A template that cannot be used fails with `{"error", "code", "params"}`: `invalid_field_name`
(`name`, `tag`), `invalid_show_if` (`field`, `detail`), `invalid_pattern` (`field`), `syntax`
(`detail`: Go template syntax that could never render), `invalid` (`detail`). Warnings come back with
the compiled template:

* `missing_label` (`field`): a field without a label.
* `show_if_unknown_field` (`field`, `name`): a show-if condition reading a field the template does not
  declare.
* `unknown_binding` (`name`, and `table` inside a table's rows): the template prints `{{.name}}` but
  no field, system value (section 5.1) or, inside the rows, column has that name. Usually a typo.
* `unused_field` (`field`): a field the form asks for that the template never prints, tests
  (`{{if}}`) or reads in a show-if.
* `duplicate_field` (`field`): one name declared as fields of different kinds; only the first
  declaration counts. Declaring the same field twice as the same kind is fine (it prints the answer
  twice).
* `fillable_ignored` (`field`, `tag`): the `fillable` attribute on a tag that cannot be filled in a
  PDF (only text, number, date and text area tags can).

`prepare` (given the normalized `fields` or the raw `schema`) checks a document's answers and returns them cleaned, with formula and row-number cells
computed and hidden fields dropped, plus a list of `{"key", "code", "params"}`: `invalid`,
`too_long` (`count`), `not_a_number`, `invalid_date`, `not_an_option`, `too_many_files` (`count`),
`too_many_rows` (`count`), `blank`, `too_small` (`count`), `too_large` (`count`). An error in a table
cell also carries `row` (from 0, in the returned rows) and `column` (the column's key). File answers are
returned as sent: what a reference may point to is the consumer's rule.

### 8.3 Versions
`rebc version` and `__rebVersion()` give the engine version; `compile` returns it as
`engineVersion`, so a consumer can record which engine compiled each template version.

### 8.4 Compatibility
Engine releases follow semantic versioning. The contract they version is:

* the `rebc` commands and the WebAssembly functions: their input and output JSON, `rebc`'s exit
  status and error object;
* both schemas (8.1) and every error, warning and field-error code with its parameters (8.2);
* the `.reb` language: the tags, attributes, column types, `show-if` conditions, template functions
  with their argument forms, and system values;
* what a template compiles and renders to: the cases in `testdata/golden` (each a `.reb` with its
  expected schema, compiled template and rendered document), `testdata/formula_cases.json` and
  `testdata/show_if_cases.json`.

Section 4.5 (fillable PDF fields: the `fillable` attribute, `{{.Fillable}}`, the `fillable` render
input, `rebc fillable`, `rebc pdf-answers`, `rebpdf.wasm` and the `fillable_ignored` warning) is
experimental and outside this contract until a release says otherwise.

The engine's Go packages (`internal/`) and the wording of messages (`error`, `message`; consumers
translate by code) are not part of it.

* A **patch** release fixes bugs. It changes output only where the output contradicted this
  specification, and its changelog entry lists each such change.
* A **minor** release adds: tags, attributes, functions, optional input, output fields, warning
  codes. Consumers ignore output fields and warning codes they do not know. A new error or
  field-error code only concerns templates that use something the release added.
* A **major** release is anything else: removing or renaming part of the contract, refusing a
  template an earlier release of the same major version accepted, or changing what such a template
  compiles or renders to.

Before v1.0.0, a minor release may also break the contract; the changelog says how.

