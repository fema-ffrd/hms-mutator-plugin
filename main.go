package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/fema-ffrd/cc-go-sdk"
	"github.com/fema-ffrd/hms-mutator/actions"
	"github.com/fema-ffrd/hms-mutator/hms"
	"github.com/fema-ffrd/hms-mutator/utils"
	"github.com/google/uuid"
)

var pluginName string = "hms-mutator"

const WORKING_DIRECTORY string = "/data"

func main() {
	err := run()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	// fmt.Println("starting the hms-mutator")
	pm, err := cc.InitPluginManager()
	if err != nil {
		return fmt.Errorf("could not initiate plugin manager: %v", err)
	}
	// get the payload.
	payload := pm.Payload
	controlStartTime := time.Now() //introduces a dependency of select random basin for single stochastic transposition. consider consolidating into one action to remove the dependency.
	for _, a := range payload.Actions {
		switch a.Type {
		case "select_random_basin":
			seedSet, err := getSeeds(payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			basinDS, err := pm.GetInputDataSource("Input_Basin_Directory")
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			outBasinDS, err := pm.GetOutputDataSource("Output_Basin_Directory")
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			srb := actions.InitSelectBasinAction(a, seedSet, basinDS, outBasinDS)

			controlStartTime, err = srb.Compute()

			if err != nil {
				return err
			}

		case "single_stochastic_transposition":
			seedSet, err := getSeeds(payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			gridFileBytes, err := getInputBytes("HMS Model", ".grid", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			metFileBytes, err := getInputBytes("HMS Model", ".met", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}

			transpositionDomainBytes, err := getInputBytes("TranspositionRegion", "", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			watershedDomainBytes, err := getInputBytes("WatershedBoundary", "", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			gridFile, err := hms.ReadGrid(gridFileBytes)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			metFile, err := hms.ReadMet(metFileBytes)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}

			sst := actions.InitSingleStochasticTransposition(pm, gridFile, metFile, seedSet, transpositionDomainBytes, watershedDomainBytes)
			bootstrapCatalogString := a.Attributes.GetStringOrDefault("bootstrap_catalog", "false")
			bootstrapCatalog, err := strconv.ParseBool(bootstrapCatalogString)
			if err != nil {
				pm.Logger.Error("could not parse bootstrap_catalog parameter")
				return err
			}
			bootstrapCatalogLength := a.Attributes.GetIntOrDefault("bootstrap_catalog_length", len(gridFile.Events))
			if len(gridFile.Events) < bootstrapCatalogLength {
				pm.Logger.Error("cannot allow bootstrap_catalog_length to be greater than the catalog length")
				return errors.New("bootstrap_catalog_length too large")
			}
			normalizeTimeShiftString := a.Attributes.GetStringOrDefault("normalize", "true")
			normalizeTimeShift, err := strconv.ParseBool(normalizeTimeShiftString)
			userSpecifiedOffset := a.Attributes.GetIntOrDefault("start_time_offset", 0)
			if err != nil {
				pm.Logger.Error("could not parse normalize parameter")
				return err
			}
			output, err := sst.Compute(bootstrapCatalog, bootstrapCatalogLength, normalizeTimeShift, controlStartTime, userSpecifiedOffset)
			if err != nil {
				pm.Logger.Error("could not compute payload")
				return err
			}
			dssGridCacheDataSource, err := pm.GetInputDataSource("DSS Grid Cache")
			if err != nil {
				pm.Logger.Error("could not find DSS Grid Cache datasource")
				return err
			}
			root := dssGridCacheDataSource.Paths["default"]
			stormName := strings.Replace(output.StormName, "\\", "/", -1)
			stormDataSource := cc.DataSource{
				Name:      "DssFile",
				ID:        &uuid.NameSpaceDNS,
				Paths:     map[string]string{"default": fmt.Sprintf("%v%v", root, stormName)},
				StoreName: dssGridCacheDataSource.StoreName,
			}
			dssBytes, err := utils.GetFile(*pm, stormDataSource, "default")
			if err != nil {
				pm.Logger.Error("could not find storm")
				return err
			}
			err = putOutputBytes(dssBytes, "Storm DSS File", payload, pm)
			if err != nil {
				pm.Logger.Error("could not put storm")
				return err
			}
			err = putOutputBytes(output.GridBytes, "Grid File", payload, pm)
			if err != nil {
				pm.Logger.Error("could not put grid file")
				return err
			}
			err = putOutputBytes(output.MetBytes, "Met File", payload, pm)
			if err != nil {
				pm.Logger.Error("could not put grid file")
				return err
			}
		case "stratified_locations":
			gridFileBytes, err := getInputBytes("HMS Model", ".grid", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}

			transpositionDomainBytes, err := getInputBytes("TranspositionRegion", "", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			watershedDomainBytes, err := getInputBytes("WatershedBoundary", "", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			gridFile, err := hms.ReadGrid(gridFileBytes)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			sla, err := actions.InitStratifiedCompute(a, gridFile, transpositionDomainBytes, watershedDomainBytes) //, payload.Outputs[0])
			if err != nil {
				pm.Logger.Error("could not initalize stratified locations for this payload")
				return err
			}
			output, err := sla.Compute()
			//put the output

			if err != nil {
				pm.Logger.Error("could not compute stratified locations for this payload")
				return err
			}
			locations, err := pm.GetOutputDataSource("Locations")
			if err != nil {
				pm.Logger.Error("could not put stratified locations for this payload")
				return err
			}
			err = utils.PutFile(output.CandiateLocations.ToBytes(), pm.IOManager, locations, "default")
			if err != nil {
				pm.Logger.Error("could not put stratified locations file")
				return err
			}
			gridFileOutput, err := pm.GetOutputDataSource("GridFile")
			if err != nil {
				pm.Logger.Error("could not put gridfiles for this payload")
				return err
			}
			root := path.Dir(gridFileOutput.Paths["default"])
			for k, v := range output.GridFiles {
				gridFileOutput.Paths["default"] = fmt.Sprintf("%v/%v.grid", root, k)
				err = utils.PutFile(v, pm.IOManager, gridFileOutput, "default")
				if err != nil {
					pm.Logger.Error("could not put grid file: " + k)
					return err
				}
			}
		case "valid_stratified_locations": //aka fishnets
			gridFileBytes, err := getInputBytes("HMS Model", ".grid", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}

			transpositionDomainBytes, err := getInputBytes("TranspositionRegion", "", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			watershedDomainBytes, err := getInputBytes("WatershedBoundary", "", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			gridFile, err := hms.ReadGrid(gridFileBytes)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			sla, err := actions.InitStratifiedCompute(a, gridFile, transpositionDomainBytes, watershedDomainBytes) //, payload.Outputs[0])
			if err != nil {
				pm.Logger.Error("could not initalize valid stratified locations for this payload")
				return err
			}
			//inputSource, err := pm.GetInputDataSource("Cumulative Grids")
			outputDataSource, err := a.GetOutputDataSource("ValidLocations")
			if err != nil {
				pm.Logger.Error("could not put valid stratified locations for this payload")
				return err
			}
			root := outputDataSource.Paths["default"]
			output, err := sla.DetermineValidLocationsQuickly(pm.IOManager) //sla.DetermineValidLocations(inputSource) //update to be based on output location?
			if err != nil {
				pm.Logger.Error("could not compute valid stratified locations for this payload")
				return err
			}

			outputDataSource.Paths["default"] = fmt.Sprintf("%v/%v.csv", root, "AllStormsAllLocations")
			outbytes := make([]byte, 0)
			outbytes = append(outbytes, "StormName,X,Y,IsValid"...)
			//create random list of ints
			indexes := make([]int, len(output.AllStormsAllLocations))
			rand := rand.New(rand.NewSource(945631))
			for i := 0; i < len(indexes); i++ {
				j := rand.Intn(i + 1)
				if i != j {
					indexes[i] = indexes[j]
				}
				indexes[j] = i
			}
			for i, _ := range output.AllStormsAllLocations {
				outbytes = append(outbytes, fmt.Sprintf("%v,%v,%v,%v\n", output.AllStormsAllLocations[indexes[i]].StormName, output.AllStormsAllLocations[indexes[i]].Coordinate.X, output.AllStormsAllLocations[indexes[i]].Coordinate.Y, output.AllStormsAllLocations[indexes[i]].IsValid)...)
			}
			err = utils.PutFile(outbytes, pm.IOManager, outputDataSource, "default")
			if err != nil {
				pm.Logger.Error("could not put valid locations file")
				return err
			}
		case "storm_typed_normal_density_locations": //aka fishnets
			gridFileBytes, err := getInputBytes("HMS Model", ".grid", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}

			transpositionDomainBytes, err := getInputBytes("TranspositionRegion", "", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			watershedDomainBytes, err := getInputBytes("WatershedBoundary", "", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			gridFile, err := hms.ReadGrid(gridFileBytes)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			sla, err := actions.InitStratifiedCompute(a, gridFile, transpositionDomainBytes, watershedDomainBytes) //, payload.Outputs[0])
			if err != nil {
				pm.Logger.Error("could not initalize locations for this payload")
				return err
			}
			err = sla.DetermineStormTypeNormalDensityKernelLocations(a.IOManager) //sla.DetermineValidLocations(inputSource) //update to be based on output location?
			if err != nil {
				pm.Logger.Error("could not compute locations for this payload")
				return err
			}
		case "normal_density_locations": //aka fishnets
			gridFileBytes, err := getInputBytes("HMS Model", ".grid", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}

			transpositionDomainBytes, err := getInputBytes("TranspositionRegion", "", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			watershedDomainBytes, err := getInputBytes("WatershedBoundary", "", payload, pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			gridFile, err := hms.ReadGrid(gridFileBytes)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
			sla, err := actions.InitStratifiedCompute(a, gridFile, transpositionDomainBytes, watershedDomainBytes) //, payload.Outputs[0])
			if err != nil {
				pm.Logger.Error("could not initalize locations for this payload")
				return err
			}
			err = sla.DetermineNormalDensityKernelLocations(a.IOManager) //sla.DetermineValidLocations(inputSource) //update to be based on output location?
			if err != nil {
				pm.Logger.Error("could not compute locations for this payload")
				return err
			}

		case "full_simulation_sst":
			sst := actions.InitFullRealizationSST(a)
			err = sst.Compute(pm)
			if err != nil {
				pm.Logger.Error(err.Error())
				return err
			}
		}
	}

	pm.Logger.Info("complete 100 percent")
	return nil
}

func getInputBytes(keyword string, extension string, payload cc.Payload, pm *cc.PluginManager) ([]byte, error) {
	returnBytes := make([]byte, 0)
	for _, input := range payload.Inputs {
		if strings.Contains(input.Name, keyword) {
			index := "default"
			has := false
			if extension != "" {
				for i, Path := range input.Paths {
					//index, _ := strconv.Atoi(i)
					if strings.Contains(Path, extension) {
						index = i
						has = true
					}
				}
			} else {
				has = true
			}
			if has {
				return utils.GetFile(*pm, input, index)
			} else {
				return returnBytes, errors.New("could not find extension " + extension)
			}

		}
	}
	return returnBytes, errors.New("could not find keyword " + keyword)
}
func putOutputBytes(data []byte, keyword string, payload cc.Payload, pm *cc.PluginManager) error {
	output, err := pm.GetOutputDataSource(keyword)
	if err != nil {
		return err
	}
	err = utils.PutFile(data, pm.IOManager, output, "default")
	if err != nil {
		return err
	}
	return nil
}

func readSeedFile(seedFileBytes []byte) (utils.SeedSet, error) {
	//read event configuration
	var ec utils.EventConfiguration
	var seedSet utils.SeedSet
	err := json.Unmarshal(seedFileBytes, &ec)
	if err != nil {
		return seedSet, err
	}
	seedSetName := pluginName
	seedinstance := ec
	seeds, ssok := seedinstance.Seeds[seedSetName]
	if !ssok {
		return seedSet, errors.New("could not find seed set for seedset name")
	}
	return seeds, nil
}

func getSeeds(payload cc.Payload, pm *cc.PluginManager) (utils.SeedSet, error) {
	var seedSet utils.SeedSet
	seedFileBytes, err := getInputBytes("seeds", "", payload, pm)
	if err != nil {
		return seedSet, err
	}
	seedSet, err = readSeedFile(seedFileBytes)
	if err != nil {
		return seedSet, err
	}
	return seedSet, err
}
