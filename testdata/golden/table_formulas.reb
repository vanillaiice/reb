<reb-table name="items" label="Items" options="no:autoincrement,item:text,qty:number,rate:number,total:formula[qty*rate|2],grade:select[A|B|C],ok:checkbox,photo:photo">
  <table>
    <thead><tr><th>No</th><th>Item</th><th>Qty</th><th>Rate</th><th>Total</th><th>Grade</th><th>OK</th><th>Photo</th></tr></thead>
    <tbody>
      <reb-row>
        <td>{{.no}}</td><td>{{.item}}</td><td>{{.qty}}</td><td>{{formatNumber .rate 2}}</td><td>{{.total}}</td>
        <td>{{if eq .grade "A"}}<b>A</b>{{else}}{{.grade}}{{end}}</td>
        <td>{{if .ok}}yes{{else}}no{{end}}</td>
        <td>{{if .photo}}<img src="{{.photo}}">{{end}}</td>
      </reb-row>
    </tbody>
  </table>
</reb-table>
<p class="total">Total: {{sumColumn .items "total"
  | formatMoney "QAR" 2}}</p>
<p>Average rate: {{divide (sumColumn .items "rate") 2 | formatNumber 3}}</p>
<p>Scaled: {{multiply (add 1 2) (subtract 10 4)}}</p>
