package exporteos

func (w *writer) writeHeader() {
	w.line("Ident 3:0")
	w.line("Manufacturer ETC")
	w.line("Console Eos")
	w.line("$$Format " + w.opts.FormatVersion)
	w.line("$$Title " + w.opts.Title)
	w.line("Clear All")
	w.line("Set Channels 5000")
	w.blank()
}
