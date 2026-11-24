package slsp

import "strings"

// SampleCSV returns a small CSV with the headings this tool reads and a few
// filled-in rows, so that somebody starting from a blank spreadsheet has
// something to copy rather than a list of column names to interpret.
//
// The rows show both kinds of taxpayer — a company named by its registered
// name and an individual named in three parts — because getting that
// distinction right is the part of a summary list people most often get wrong.
func SampleCSV(k ListKind) string {
	if k == Purchases {
		return strings.Join([]string{
			"TIN,Type,Registered Name,Surname,First Name,Middle Name,Address 1,Address 2,Exempt Purchases,Zero-Rated Purchases,Taxable Services,Taxable Capital Goods,Taxable Other Goods,Input Tax",
			`005-396-840-000,N,Lopez Power Distribution,,,,Ortigas Avenue,Pasig City,0,0,48000.00,0,0,5760.00`,
			`123-456-789-000,N,Pilipinas Paper Supply Inc.,,,,41 Bagumbayan Street,Quezon City,0,0,0,0,25000.00,3000.00`,
			`234-567-890-000,I,,DELA CRUZ,JUAN,PEREZ,18 Mabini Street,Caloocan City,0,0,12000.00,0,0,1440.00`,
			`345-678-901-002,N,Southern Hardware Trading,,,,Osmena Boulevard,Cebu City,0,0,0,80000.00,0,9600.00`,
			`456-789-012-000,N,Peña Water District,,,,Poblacion,Los Banos Laguna,3500.00,0,0,0,0,0`,
			"",
		}, "\r\n")
	}
	return strings.Join([]string{
		"TIN,Type,Registered Name,Surname,First Name,Middle Name,Address 1,Address 2,Exempt Sales,Zero-Rated Sales,Taxable Sales,Output Tax",
		`123-456-789-000,N,Acme Trading Corporation,,,,12 Ayala Avenue,Makati City,0,0,100000.00,12000.00`,
		`234-567-890-001,N,Bayan Supply Inc.,,,,Osmena Boulevard,Cebu City,0,0,50000.00,6000.00`,
		`345-678-901-000,I,,SANTOS,MARIA CLARA,REYES,7 Rizal Street,Davao City,0,0,18000.00,2160.00`,
		`456-789-012-000,N,Pacific Freight Forwarders,,,,Pier 15 North Harbour,Manila,0,75000.00,0,0`,
		`567-890-123-000,N,Saint Jude Medical Clinic,,,,88 Quezon Avenue,Quezon City,22000.00,0,0,0`,
		"",
	}, "\r\n")
}

// SampleCSVName is the name the sample is offered under.
func SampleCSVName(k ListKind) string {
	if k == Purchases {
		return "summary-list-of-purchases-template.csv"
	}
	return "summary-list-of-sales-template.csv"
}
