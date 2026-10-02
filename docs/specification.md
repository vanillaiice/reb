# Rebar `.reb` Template Format: Technical Specification & Compiler Rules

This document provides a formal technical specification of the Rebar (`.reb`) template format. It is designed to act as a definitive reference for engineers authoring parsers, compiling services, and AI systems generating valid Rebar templates.

---

## 1. Architectural Design Goal
The `.reb` (Rebar Template Layout) format is a hybrid declarative markup structure. Its primary goal is to **unify input schemas and presentation layouts in a single file**, eliminating synchronization drift between web database schemas and printed PDF documents. 

A `.reb` template is compiled by the Go backend (and frontend editor) into:
1. **JSON Schema Array**: Extracted from custom `<reb-*>` elements and used by the web client to render active forms dynamically.
2. **Go Template HTML Structure**: Custom tags are transpiled into standard HTML with `{{.Answers...}}` data bindings used by the headless Chromium compiler (Gotenberg) to output high-fidelity vector PDFs.

---

## 2. File Topology
A standard `.reb` file uses standard HTML5 markup combined with custom Rebar form elements.

```mermaid
graph TD
    A[Rebar .reb File] --> B[Standard HTML Structure]
    A --> C[Presentation Style: &lt;style&gt;]
    A --> D[Custom Form Elements: &lt;reb-*&gt;]
    D --> E[Compiled to JSON Schema Array]
    D --> F[Transpiled to Go {{.Answers.*}} Bindings]
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
* `<reb-text>`: Standard single-line text input.
* `<reb-number>`: Numeric parameters.
* `<reb-textarea>`: Multi-line text block.
* `<reb-date>`: Interactive calendar date-picker.
* `<reb-select>`: Dropdown selection box.
* `<reb-photogrid>`: Renders an upload zone for multiple photos.
* `<reb-signature>`: Digital signature pad capturing canvas strokes as SVG.

### Element Attributes:
Every `<reb-*>` element supports the following attributes:
* **`name`** (`string`, Required): The unique alphanumeric identifier for the field. Used as the binding key in the JSON schema and Go template. Must match regex `^[A-Za-z_][A-Za-z0-9_]*$` (letters, digits and underscores, not starting with a digit); the compiler rejects anything else, because the name becomes a `{{.name}}` template binding.
* **`label`** (`string`, Required): The human-readable label rendered next to the input field in the web client.
* **`options`** (`comma-separated string`, Optional): Mandated only when using `<reb-select>`. Represents allowed selection values (e.g., `options="High,Medium,Low"`).
* **`class`** (`string`, Optional): Standard CSS/Tailwind classes to apply to the output element during PDF generation.

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

**Example Grand Total:**
```html
<div class="text-right font-bold text-lg">
  Grand Total: ${{sumColumn .defects "amount" | formatNumber 2}}
</div>
```

---

## 5. Tokenization & Parsing Strategy (For Parsers & AI)
When compiling a `.reb` template, compilers or AI parsers should follow this process:

### Phase 1: AST Extraction
Parse the document using an HTML/XML parser (like Go's `golang.org/x/net/html` or JS's `DOMParser`). Traverse the DOM tree looking for elements starting with `reb-`.

### Phase 2: Schema Generation
For every matched `<reb-*>` node, extract the tag type (omitting `reb-`), `name`, `label`, and `options`. Push these into a structured JSON array representing the fields schema.

### Phase 3: Transpilation
Mutate the `<reb-*>` nodes in the AST in-place. Change the node tag to `span` (or `div`), strip Rebar-specific attributes (preserving `class`), and inject the `{{.NAME}}` Go template syntax as text nodes. Serialize the AST back to a string. Note: `<reb-declare>` tags must be completely removed from the AST.

---

## 6. Complete Valid File Example

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
