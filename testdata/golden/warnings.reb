<reb-text name="site"></reb-text>
<reb-number name="site" label="Site again"></reb-number>
<reb-text name="never_printed" label="Never printed"></reb-text>
<reb-declare name="watched" label="Watched" show-if="ghost_field"></reb-declare>
<reb-table name="rows" label="Rows" options="a:number,b:text">
  <table><tr reb-row><td>{{.a}}</td><td>{{.c}}</td><td>{{$.site}}</td></tr></table>
</reb-table>
<p>{{.sitee}} {{.Answers.watched}} {{with .site}}{{.anything}}{{end}}</p>
