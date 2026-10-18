<reb-number name="qty" label="Total quantity" />
<reb-text name="site" label="Site" />

<reb-table name="materials" label="Materials" options="item:text,qty:number">
  <reb-declare name="qty" type="number" />
  <table>
    <tr reb-row><td>{{.item}}</td><td>{{.qty}}</td></tr>
  </table>
</reb-table>

<reb-table name="labour" label="Labour" options="item:text,hours:number">
  <table>
    <tr reb-row><td>{{.item}}</td><td>{{.hours}}</td></tr>
  </table>
</reb-table>

<p>{{.site}}: {{.qty}} in all</p>
