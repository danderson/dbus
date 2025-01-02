package dbus

type Description struct {
	Name       string `xml:"name,attr"`
	Interfaces []struct {
		Name    string `xml:"name,attr"`
		Methods []struct {
			Name      string `xml:"name,attr"`
			Arguments []struct {
				Name      string `xml:"name,attr"`
				Type      string `xml:"type,attr"`
				Direction string `xml:"direction,attr"`
			} `xml:"arg"`
			Annotations []struct {
				Name  string `xml:"name,attr"`
				Value string `xml:"value,attr"`
			} `xml:"annotation"`
		} `xml:"method"`
		Signals []struct {
			Name       string `xml:"name,attr"`
			Attributes struct {
				Name string `xml:"name,attr"`
				Type string `xml:"type,attr"`
			} `xml:"arg"`
		} `xml:"signal"`
		Properties []struct {
			Name   string `xml:"name,attr"`
			Type   string `xml:"type,attr"`
			Access string `xml:"access,attr"`
		} `xml:"property"`
	} `xml:"interface"`
	Children []*Description `xml:"node"`
}
