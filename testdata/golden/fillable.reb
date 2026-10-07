<!-- Fillable fields: rendered with "fillable": true they become empty boxes that rebpdf turns into
     PDF text fields; otherwise they print their answers. -->
<style>
  body { font-family: Arial, sans-serif; font-size: 14px; }
  .row { margin: 12px 0; }
  .wide { width: 18em; }
</style>

<h1>Material Request</h1>
{{if .Fillable}}<p class="hint">Type into the boxes, save the PDF and send it back.</p>{{end}}

<div class="row">Requested by: <strong>{{.ReporterName}}</strong></div>
<div class="row">Supplier: <reb-text name="supplier" label="Supplier" fillable class="wide"></reb-text></div>
<div class="row">Quantity: <reb-number name="quantity" label="Quantity" fillable></reb-number></div>
<div class="row">Delivery date: <reb-date name="delivery" label="Delivery date" fillable></reb-date></div>
<div class="row">Priority: <reb-select name="priority" label="Priority" options="Normal,Urgent"></reb-select></div>

<h2>Remarks</h2>
<reb-textarea name="remarks" label="Remarks" fillable></reb-textarea>

<p>Supplier again: <reb-text name="supplier" label="Supplier" fillable></reb-text></p>
