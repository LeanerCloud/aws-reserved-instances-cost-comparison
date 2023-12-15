package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"

	"os"
	"strings"

	ec2instancesinfo "github.com/LeanerCloud/ec2-instances-info"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/olekukonko/tablewriter"
)

const (
	HoursInMonth = 730 // Average hours in a month
)

// Log function for simple logging
const (
	Info = iota
	Error
	Debug
)

var LogLevel = Info

// Define logger instances for different log levels
var (
	debugLog = log.New(os.Stdout, "DEBUG: ", log.LstdFlags|log.Lshortfile)
	errorLog = log.New(os.Stderr, "ERROR: ", log.LstdFlags|log.Lshortfile)
)

type InstanceInfo struct {
	InstanceType      string
	NumberOfInstances int
	Engine            string
}

type PricingData struct {
	AmortizedMonthlyCostPerInstance float64
	Engine                          string
	InstanceType                    string
	MonthlyCostPerInstance          float64
	NumberOfInstances               int
	PaymentOption                   string
	Region                          string
	Savings                         float64
	SavingsPercent                  float64
	Term                            string
	TotalUpfrontCost                float64
	TotalAmortizedMonthlyCost       float64
	TotalCostForTerm                float64
	CostForTermPerInstance          float64
	TotalMonthlyCost                float64
	UpfrontCost                     float64
}

type InstancePricing struct {
	OneYear   []PricingData
	ThreeYear []PricingData
}

var (
	Region string
)

type logWriter struct {
	level    int
	original io.Writer
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	if LogLevel >= w.level {
		return w.original.Write(p)
	}
	return len(p), nil // Pretend to write, but actually discard
}

func FetchAndProcessRDSData(region string, instances []InstanceInfo) ([]ec2instancesinfo.RDSInstance, error) {
	rdsData, err := ec2instancesinfo.RDSData()
	if err != nil {
		errorLog.Printf("Error fetching RDS data: %v", err)
		return nil, err
	}
	debugLog.Printf("Fetched RDS data successfully")

	var filteredInstances []ec2instancesinfo.RDSInstance
	for _, instance := range instances {

		for _, data := range *rdsData {
			if pricing, ok := data.Pricing[region]; ok {
				switch instance.Engine {
				case "MySQL":
					if pricing.MySQL.OnDemand != 0 {
						filteredInstances = append(filteredInstances, data)
					}
				case "PostgreSQL":
					if pricing.PostgreSQL.OnDemand != 0 {
						filteredInstances = append(filteredInstances, data)
					}
					// Add cases for other database engines as needed
				}
			}
		}
	}
	debugLog.Printf("Total filtered instances: %d", len(filteredInstances))
	return filteredInstances, nil
}

// ProcessOnDemand processes on-demand pricing data and returns two PricingData structs.
func ProcessOnDemand(instance ec2instancesinfo.RDSInstance, region string, service string, numberOfInstances int) (PricingData, PricingData) {
	var onDemandPrice float64

	// Extract on-demand pricing based on service
	switch service {
	case "MySQL":
		onDemandPrice = instance.Pricing[region].MySQL.OnDemand
	case "PostgreSQL":
		onDemandPrice = instance.Pricing[region].PostgreSQL.OnDemand
		// Add cases for other database engines as needed
	}

	// Debug: Ensure that the onDemandPrice is correctly fetched
	debugLog.Printf("On-Demand Price for %s in region %s: %f", service, region, onDemandPrice)

	// Calculate monthly cost considering the number of instances
	monthlyCost := onDemandPrice * float64(HoursInMonth)
	debugLog.Printf("Monthly Cost for %s in region %s: %f", service, region, monthlyCost)

	// Calculate costs for 1-year and 3-year terms
	totalCostForTerm1Year := monthlyCost * 12
	totalCostForTerm3Years := monthlyCost * 36

	// Create PricingData structs for 1-year and 3-year terms
	data1Year := PricingData{
		Region:                          region,
		InstanceType:                    instance.InstanceType,
		AmortizedMonthlyCostPerInstance: monthlyCost,
		NumberOfInstances:               numberOfInstances,
		Term:                            "On-Demand",
		PaymentOption:                   "N/A",
		UpfrontCost:                     0,
		MonthlyCostPerInstance:          onDemandPrice * float64(HoursInMonth),
		TotalCostForTerm:                totalCostForTerm1Year * float64(numberOfInstances),
		CostForTermPerInstance:          totalCostForTerm1Year,
		Savings:                         0,
		SavingsPercent:                  0,
		TotalUpfrontCost:                0,
		TotalMonthlyCost:                monthlyCost * float64(numberOfInstances),
		TotalAmortizedMonthlyCost:       monthlyCost * float64(numberOfInstances),
	}

	data3Years := data1Year

	data3Years.CostForTermPerInstance = totalCostForTerm3Years
	data3Years.TotalCostForTerm = totalCostForTerm3Years * float64(numberOfInstances)

	return data1Year, data3Years
}

// GetRunningRdsInstances fetches running RDS instances
func GetRunningRdsInstances(region string) ([]InstanceInfo, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		errorLog.Printf("Error loading AWS config: %v", err)
		return nil, err
	}

	svc := rds.NewFromConfig(cfg)
	input := &rds.DescribeDBInstancesInput{}

	result, err := svc.DescribeDBInstances(context.TODO(), input)
	if err != nil {
		errorLog.Printf("Error describing RDS instances: %v", err)
		return nil, err
	}

	var instances []InstanceInfo
	for _, dbInstance := range result.DBInstances {
		if *dbInstance.DBInstanceStatus == "available" {
			instances = append(instances, InstanceInfo{
				InstanceType:      *dbInstance.DBInstanceClass,
				NumberOfInstances: 1,
				Engine:            determineServiceFromDBEngine(dbInstance.Engine),
			})
		}
	}

	debugLog.Printf("Found running instances: %v", instances)
	return instances, nil
}

func determineServiceFromDBEngine(engine *string) string {
	// Simple mapping, can be expanded as needed
	switch *engine {
	case "mysql":
		return "MySQL"
	case "postgres":
		return "PostgreSQL"
	// Add other cases as needed
	default:
		return "Unknown"
	}
}

func ProcessReservedOption(instanceType, term string, amortizedHourlyCost float64, hoursInMonth int, onDemandHourly float64, numberOfInstances int) PricingData {
	termYears := 1
	if strings.Contains(term, "yrTerm3") {
		termYears = 3
	}
	monthsInTerm := 12 * termYears
	paymentOption := strings.Split(term, ".")[1]

	amortizedMonthlyCost := amortizedHourlyCost * float64(hoursInMonth)
	upfrontCost, monthlyCost := 0.0, amortizedMonthlyCost

	if paymentOption == "partialUpfront" {
		upfrontCost = amortizedMonthlyCost * float64(monthsInTerm) * 0.5
		monthlyCost = amortizedMonthlyCost * 0.5
	} else if paymentOption == "allUpfront" {
		upfrontCost = amortizedMonthlyCost * float64(monthsInTerm)
		monthlyCost = 0
	}

	totaCostForTermPerInstance := amortizedMonthlyCost * float64(monthsInTerm)
	totalCostForTerm := totaCostForTermPerInstance * float64(numberOfInstances) // calculate total cost for all instances

	totalOnDemandCost := onDemandHourly * float64(HoursInMonth) * float64(monthsInTerm)
	savings := totalOnDemandCost - totaCostForTermPerInstance
	savingsPercent := 0.0
	if totalOnDemandCost != 0 {
		savingsPercent = (savings / totalOnDemandCost) * 100
	}

	debugLog.Printf("Processed ReservedOption for instance: %s, term: %s", instanceType, term)
	return PricingData{
		Region:                          Region,
		InstanceType:                    instanceType,
		AmortizedMonthlyCostPerInstance: amortizedMonthlyCost,
		NumberOfInstances:               numberOfInstances,
		Term:                            fmt.Sprintf("%d Year", termYears),
		PaymentOption:                   paymentOption,
		UpfrontCost:                     upfrontCost,
		MonthlyCostPerInstance:          monthlyCost,
		CostForTermPerInstance:          totaCostForTermPerInstance,
		Savings:                         savings,
		SavingsPercent:                  savingsPercent,
		TotalUpfrontCost:                upfrontCost * float64(numberOfInstances),
		TotalMonthlyCost:                monthlyCost,
		TotalAmortizedMonthlyCost:       0,
		TotalCostForTerm:                totalCostForTerm,
	}
}

// // PrintMarkdownTable prints the data in a markdown table format using tablewriter.
func PrintMarkdownTable(data []PricingData, columns []string, title string) {
	fmt.Println("\n" + title)
	table := tablewriter.NewWriter(os.Stdout)
	header := make([]string, 0, len(columns))

	header = append(header, columns...)

	table.SetHeader(header)
	//table.SetAlignment()
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")

	for _, row := range data {
		table.Append(convertPricingDataToSlice(row, columns))
	}

	table.Render()
}

func convertPricingDataToSlice(data PricingData, columns []string) []string {
	var row []string
	for _, col := range columns {
		switch col {
		case "Region":
			row = append(row, data.Region)
		case "Instance Type":
			row = append(row, data.InstanceType)
		case "Amortized Monthly Cost/instance ($)":
			row = append(row, fmt.Sprintf("%.2f", data.AmortizedMonthlyCostPerInstance)) //
		case "Number of Instances":
			row = append(row, fmt.Sprintf("%d", data.NumberOfInstances))
		case "Term":
			row = append(row, data.Term)
		case "Payment Option":
			row = append(row, data.PaymentOption)
		case "Upfront Cost / instance ($)":
			row = append(row, fmt.Sprintf("%.2f", data.UpfrontCost))
		case "Monthly Cost / instance ($)":
			row = append(row, fmt.Sprintf("%.2f", data.MonthlyCostPerInstance))
		case "Total Cost for Term / instance ($)":
			row = append(row, fmt.Sprintf("%.2f", data.CostForTermPerInstance))
		case "Savings ($)":
			row = append(row, fmt.Sprintf("%.2f", data.Savings))
		case "Savings (%)":
			row = append(row, fmt.Sprintf("%.2f", data.SavingsPercent))
		case "Total Upfront Cost ($)":
			row = append(row, fmt.Sprintf(" %.2f", data.TotalUpfrontCost))
		case "Total Monthly Cost ($)":
			row = append(row, fmt.Sprintf(" %.2f", data.TotalMonthlyCost))
		case "Monthly Cost /instance ($)":
			row = append(row, fmt.Sprintf("%.2f", data.MonthlyCostPerInstance)) //
		case "Total Amortized Monthly Cost ($)":
			row = append(row, fmt.Sprintf("%.2f", data.TotalAmortizedMonthlyCost))
		case "Total Cost for Term ($)":
			row = append(row, fmt.Sprintf("%.2f", data.TotalCostForTerm))
			// Add other cases as needed based on your PricingData struct fields
		}
	}
	return row
}

// AggregateCosts aggregates costs based on instance counts and returns PricingData structs.
func AggregateCosts(data []PricingData, instances []InstanceInfo) []PricingData {
	instanceCounts := make(map[string]int)
	for _, instance := range instances {
		key := fmt.Sprintf("%s-%s", instance.InstanceType, instance.Engine)
		instanceCounts[key] += instance.NumberOfInstances
	}

	var aggregatedData []PricingData
	for _, row := range data {
		key := fmt.Sprintf("%s-%s", row.InstanceType, row.Engine)
		if count, exists := instanceCounts[key]; exists {
			newRow := row // Copy struct
			newRow.NumberOfInstances = count

			newRow.TotalMonthlyCost = newRow.MonthlyCostPerInstance * float64(count)
			newRow.TotalAmortizedMonthlyCost = newRow.AmortizedMonthlyCostPerInstance * float64(count)

			aggregatedData = append(aggregatedData, newRow)
		}
	}
	return aggregatedData
}

func AggregateCostsByTermAndEngine(data []PricingData, instances []InstanceInfo, term, engine string) []PricingData {
	var aggregatedData []PricingData

	debugLog.Printf("Aggregated Data before: %v", data)
	for _, d := range data {
		if d.Engine == engine && (d.Term == term || d.Term == "On-Demand") {
			count := getInstanceCount(d.InstanceType, engine, instances)
			if count > 0 {
				d.NumberOfInstances = count
				d.TotalMonthlyCost = d.MonthlyCostPerInstance * float64(count)
				d.TotalAmortizedMonthlyCost = d.AmortizedMonthlyCostPerInstance * float64(count)
				if d.Term != "On-Demand" {
					d.TotalCostForTerm = d.CostForTermPerInstance * float64(count)
				}
				aggregatedData = append(aggregatedData, d)
			}
		}
	}
	debugLog.Printf("Aggregated Data After: %v", aggregatedData)
	return aggregatedData
}

func ProcessInstanceTypes(instanceTypeData []ec2instancesinfo.RDSInstance, region string, runningInstances []InstanceInfo) ([]PricingData, []PricingData) {
	processed := make(map[string]bool) // To track processed instance types
	var finalData1Year, finalData3Years []PricingData

	for _, runningInstance := range runningInstances {
		key := fmt.Sprintf("%s-%s", runningInstance.InstanceType, runningInstance.Engine)
		if processed[key] {
			continue // Skip if already processed
		}

		for _, instanceData := range instanceTypeData {
			if instanceData.InstanceType == runningInstance.InstanceType {
				// Process only if the instance type matches a running instance
				data1Year, data3Years := ProcessInstanceType(instanceData, region, runningInstance.Engine, runningInstance.NumberOfInstances)
				finalData1Year = append(finalData1Year, data1Year...)
				finalData3Years = append(finalData3Years, data3Years...)
			}
		}

		processed[key] = true
	}

	debugLog.Printf("Final Data 1 year: %v", finalData1Year)
	debugLog.Printf("Final Data 3 years: %v", finalData3Years)

	return finalData1Year, finalData3Years
}

func ProcessInstanceType(instance ec2instancesinfo.RDSInstance, region string, engine string, numberOfInstances int) ([]PricingData, []PricingData) {
	var data1Year, data3Years []PricingData

	// Process on-demand pricing with the updated number of instances
	onDemandData1Year, onDemandData3Years := ProcessOnDemand(instance, region, engine, numberOfInstances)

	debugLog.Printf("On-Demand Data for Instance Type %s: %+v", instance.InstanceType, onDemandData1Year)

	data1Year = append(data1Year, onDemandData1Year)
	data3Years = append(data3Years, onDemandData3Years)

	// Process reserved pricing if available
	if servicePricing, ok := instance.Pricing[region]; ok {
		var serviceRDSPricing ec2instancesinfo.RDSPricing
		switch engine {
		case "MySQL":
			serviceRDSPricing = servicePricing.MySQL

		case "PostgreSQL":
			serviceRDSPricing = servicePricing.PostgreSQL
			// Add cases for other services
		}

		// Manually process each reserved pricing option
		reservedOptions := []struct {
			Term  string
			Price float64
		}{
			{"yrTerm1Standard.noUpfront", serviceRDSPricing.Reserved.StandardNoUpfront1Year},
			{"yrTerm3Standard.noUpfront", serviceRDSPricing.Reserved.StandardNoUpfront3Years},
			{"yrTerm1Standard.partialUpfront", serviceRDSPricing.Reserved.StandardPartiallUpfront1Year},
			{"yrTerm3Standard.partialUpfront", serviceRDSPricing.Reserved.StandardPartialUpfront3Years},
			{"yrTerm1Standard.allUpfront", serviceRDSPricing.Reserved.StandardAllUpfront1Year},
			{"yrTerm3Standard.allUpfront", serviceRDSPricing.Reserved.StandardAllUpfront3Years},
			{"yrTerm1Convertible.noUpfront", serviceRDSPricing.Reserved.ConvertibleNoUpfront1Year},
			{"yrTerm3Convertible.noUpfront", serviceRDSPricing.Reserved.ConvertibleNoUpfront3Years},
			{"yrTerm1Convertible.partialUpfront", serviceRDSPricing.Reserved.ConvertiblePartiallUpfront1Year},
			{"yrTerm3Convertible.partialUpfront", serviceRDSPricing.Reserved.ConvertiblePartialUpfront3Years},
			{"yrTerm1Convertible.allUpfront", serviceRDSPricing.Reserved.ConvertibleAllUpfront1Year},
			{"yrTerm3Convertible.allUpfront", serviceRDSPricing.Reserved.ConvertibleAllUpfront3Years},
		}

		for _, option := range reservedOptions {
			if option.Price != 0 {
				reservedRow := ProcessReservedOption(instance.InstanceType, option.Term, option.Price, HoursInMonth, serviceRDSPricing.OnDemand, numberOfInstances)
				if strings.Contains(option.Term, "yrTerm1") {
					data1Year = append(data1Year, reservedRow)
				} else {
					data3Years = append(data3Years, reservedRow)
				}
			}
		}

		debugLog.Printf("Reserved pricing options: %v", reservedOptions)
	} else {
		debugLog.Printf("No pricing available for the specified service and region")
	}

	for i := range data1Year {
		data1Year[i].Engine = engine
	}
	for i := range data3Years {
		data3Years[i].Engine = engine
	}

	debugLog.Printf("Data 1 year: %v", data1Year)
	debugLog.Printf("Data 3 years: %v", data3Years)

	return data1Year, data3Years
}
func PrintTablesByInstanceTypeAndEngine(columns []string, allData []PricingData, engine string) {
	processedKeys := make(map[string]bool) // To track processed instance types with term and payment option

	for _, data := range allData {
		if data.Engine != engine {
			continue // Skip if engine does not match
		}

		key := fmt.Sprintf("%s-%s-%s-%s", data.InstanceType, data.Term, data.PaymentOption, engine)
		if processedKeys[key] {
			continue // Skip if already processed
		}

		filteredData := filterDataByInstanceTypeEngineTermAndOption(allData, data.InstanceType, engine, data.Term, data.PaymentOption)
		if len(filteredData) > 0 {
			PrintMarkdownTable(filteredData, columns, fmt.Sprintf("## Costs for %s", key))
			processedKeys[key] = true
		}
	}
}

func filterDataByInstanceTypeEngineTermAndOption(data []PricingData, instanceType, engine, term, paymentOption string) []PricingData {
	var filteredData []PricingData
	for _, d := range data {
		if d.InstanceType == instanceType && d.Engine == engine && d.Term == term && d.PaymentOption == paymentOption {
			filteredData = append(filteredData, d)
		}
	}
	// Ensure only one entry per instance type and payment option
	if len(filteredData) > 1 {
		filteredData = filteredData[:1]
	}
	return filteredData
}

// // AggregateCostsByTermAndEngine aggregates costs based on instance counts, term length, and engine

// getInstanceCount returns the total count of instances for a given type and engine.
func getInstanceCount(instanceType, engine string, instances []InstanceInfo) int {
	count := 0
	for _, instance := range instances {
		if instance.InstanceType == instanceType && instance.Engine == engine {
			count += instance.NumberOfInstances
		}
	}
	return count
}

func aggregateInstances(instances []InstanceInfo) []InstanceInfo {
	aggregated := make(map[string]InstanceInfo)
	for _, instance := range instances {
		key := fmt.Sprintf("%s-%s", instance.InstanceType, instance.Engine)
		if agg, exists := aggregated[key]; exists {
			agg.NumberOfInstances += instance.NumberOfInstances
			aggregated[key] = agg
		} else {
			aggregated[key] = instance
		}
	}

	var aggregatedList []InstanceInfo
	for _, instance := range aggregated {
		aggregatedList = append(aggregatedList, instance)
	}
	return aggregatedList
}

func InitializeLogger() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	debugLog.SetOutput(&logWriter{level: Debug, original: os.Stdout})
	errorLog.SetOutput(&logWriter{level: Error, original: os.Stderr})
}

func ParseFlags() {
	flag.StringVar(&Region, "region", "", "AWS region")
	logLevelFlag := flag.String("logLevel", "info", "Log level (debug, info, error)")
	flag.Parse()

	switch strings.ToLower(*logLevelFlag) {
	case "debug":
		LogLevel = Debug
	case "error":
		LogLevel = Error
	default:
		LogLevel = Info
	}

}

func FetchAndAggregateInstances(region string) ([]InstanceInfo, error) {
	instanceInfos, err := GetRunningRdsInstances(region)
	if err != nil {
		return nil, err
	}
	return aggregateInstances(instanceInfos), nil
}

func ProcessPricingData(region string, runningInstances []InstanceInfo) ([]PricingData, []PricingData) {
	instanceTypeData, err := FetchAndProcessRDSData(region, runningInstances)
	if err != nil {
		errorLog.Printf("Failed to fetch RDS data: %v", err)
		return nil, nil
	}

	return ProcessInstanceTypes(instanceTypeData, region, runningInstances)
}

func ProcessReservedPricing(instance ec2instancesinfo.RDSInstance, region string, engine string, numberOfInstances int) ([]PricingData, []PricingData) {
	var data1Year, data3Years []PricingData
	if servicePricing, ok := instance.Pricing[region]; ok {
		var serviceRDSPricing ec2instancesinfo.RDSPricing
		switch engine {
		case "MySQL":
			serviceRDSPricing = servicePricing.MySQL
		case "PostgreSQL":
			serviceRDSPricing = servicePricing.PostgreSQL
			// Add cases for other database engines as needed
		}

		reservedOptions := []struct {
			Term  string
			Price float64
		}{
			{"yrTerm1Standard.noUpfront", serviceRDSPricing.Reserved.StandardNoUpfront1Year},
			{"yrTerm3Standard.noUpfront", serviceRDSPricing.Reserved.StandardNoUpfront3Years},
			{"yrTerm1Standard.partialUpfront", serviceRDSPricing.Reserved.StandardPartiallUpfront1Year},
			{"yrTerm3Standard.partialUpfront", serviceRDSPricing.Reserved.StandardPartialUpfront3Years},
			{"yrTerm1Standard.allUpfront", serviceRDSPricing.Reserved.StandardAllUpfront1Year},
			{"yrTerm3Standard.allUpfront", serviceRDSPricing.Reserved.StandardAllUpfront3Years},
			{"yrTerm1Convertible.noUpfront", serviceRDSPricing.Reserved.ConvertibleNoUpfront1Year},
			{"yrTerm3Convertible.noUpfront", serviceRDSPricing.Reserved.ConvertibleNoUpfront3Years},
			{"yrTerm1Convertible.partialUpfront", serviceRDSPricing.Reserved.ConvertiblePartiallUpfront1Year},
			{"yrTerm3Convertible.partialUpfront", serviceRDSPricing.Reserved.ConvertiblePartialUpfront3Years},
			{"yrTerm1Convertible.allUpfront", serviceRDSPricing.Reserved.ConvertibleAllUpfront1Year},
			{"yrTerm3Convertible.allUpfront", serviceRDSPricing.Reserved.ConvertibleAllUpfront3Years},
		}

		for _, option := range reservedOptions {
			if option.Price != 0 {
				reservedRow := ProcessReservedOption(instance.InstanceType, option.Term, option.Price, HoursInMonth, serviceRDSPricing.OnDemand, numberOfInstances)
				if strings.Contains(option.Term, "yrTerm1") {
					data1Year = append(data1Year, reservedRow)
				} else {
					data3Years = append(data3Years, reservedRow)
				}
			}
		}

		// Set the engine for the processed data
		for i := range data1Year {
			data1Year[i].Engine = engine
		}
		for i := range data3Years {
			data3Years[i].Engine = engine
		}
	} else {
		debugLog.Printf("No reserved pricing available for the specified service and region")
	}

	return data1Year, data3Years
}

func PrintPricingTables(data []PricingData, instances []InstanceInfo, term string) {
	enginesInUse := make(map[string]bool)
	for _, instanceInfo := range instances {
		enginesInUse[instanceInfo.Engine] = true
	}

	columns := []string{
		"Region",
		"Instance Type",
		"Amortized Monthly Cost/instance ($)",
		"Number of Instances",
		"Term", "Payment Option",
		"Upfront Cost / instance ($)",
		"Monthly Cost / instance ($)",
		"Total Cost for Term / instance ($)",
		"Savings ($)",
		"Savings (%)",
		"Total Upfront Cost ($)",
		"Total Monthly Cost ($)",
		"Total Amortized Monthly Cost ($)",
		"Total Cost for Term ($)",
	}

	for engine := range enginesInUse {
		aggregatedData := AggregateCostsByTermAndEngine(data, instances, term, engine)
		title := fmt.Sprintf("## %s Term Costs for %s", term, engine)
		PrintMarkdownTable(aggregatedData, columns, title)
		//PrintMarkdownTable(data, columns, title)
	}
}

func main() {
	InitializeLogger()
	ParseFlags()

	if Region == "" {
		fmt.Println("Usage: script -region <region> [-logLevel debug/info/error]")
		os.Exit(1)
	}

	aggregatedInstances, err := FetchAndAggregateInstances(Region)
	if err != nil {
		errorLog.Printf("Failed to process instances: %v", err)
		return
	}

	pricingData1Year, pricingData3Years := ProcessPricingData(Region, aggregatedInstances)

	debugLog.Printf("Main Data 1 year: %v", pricingData1Year)
	debugLog.Printf("Main Data 3 years: %v", pricingData3Years)

	PrintPricingTables(pricingData1Year, aggregatedInstances, "1 Year")
	PrintPricingTables(pricingData3Years, aggregatedInstances, "3 Year")
}
