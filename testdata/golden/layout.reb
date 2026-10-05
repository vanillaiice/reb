<style>
  .urgent { color: #b91c1c; }
</style>
<!-- comments are removed, including <reb-text name="ghost" label="Ghost" /> -->
<reb-tailwind></reb-tailwind>
<reb-header class="text-xs"><span>{{.ProjectName}}</span></reb-header>
<reb-declare type="section" name="part_a" label="Part A" />
<reb-declare name="site" label="Site" />
<reb-declare type="checkbox" name="urgent" label="Urgent" />
<h1 {{if .urgent}}class="urgent"{{end}}>Inspection at {{.site}}</h1>
<p>Inspector: <reb-text name="inspector" label="Inspector" class="font-bold" /></p>
<reb-checkbox name="isolated" label="Isolated"></reb-checkbox>
<reb-radio name="shift" label="Shift" options="Day, Night"></reb-radio>
<reb-pagebreak></reb-pagebreak>
<reb-photogrid name="photos" label="Photos" class="grid grid-cols-2"></reb-photogrid>
<reb-attachments name="drawings" label="Drawings"></reb-attachments>
<reb-signature name="sig" label="Signature" />
<reb-footer><div>Page <span class="pageNumber"></span> of <span class="totalPages"></span></div></reb-footer>
