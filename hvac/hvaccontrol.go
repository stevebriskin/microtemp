package hvac

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"time"

	"github.com/stevebriskin/microtemp"
	"go.mongodb.org/mongo-driver/bson"
	"go.uber.org/zap"
	"go.viam.com/rdk/components/generic"
	"go.viam.com/rdk/robot/client"
	"go.viam.com/utils/rpc"

	appd "go.viam.com/api/app/data/v1"
)

// TODO: extract hvac to temp sensor mapping from app.viam.com

type HvacMachine struct {
	MachineId  string `json:"machine_id"` // for debugging only?
	MachineURI string `json:"machine_uri"`
}

type HvacZone struct {
	Hvacs        []HvacMachine `json:"hvacs"`
	TempMachines []string      `json:"temp_machines"`
	TargetTempC  float64       `json:"target_temp_c"`

	// Is it currently "heat" or "cool" mode
	HvacMode string `json:"hvac_mode"`
}

type Config struct {
	Zones []HvacZone `json:"zones"`

	// Where the temp data is
	AppOrganizationId string `json:"app_org_id"`
	AppAPIName        string `json:"app_api_name"`
	AppAPIKey         string `json:"app_api_key"`

	// Assuming a single key can access all hvac machines
	HvacAPIName string `json:"hvac_api_name"`
	HvacAPIKey  string `json:"hvac_api_key"`
}

var Conf Config

func DoStuff(ctx context.Context, logger *zap.SugaredLogger) {
	// TODO: parallelize
	for _, zone := range Conf.Zones {
		temp, err := getAverageTempFromApp(ctx, zone.TempMachines, logger)
		if err != nil {
			logger.Error(err)
			continue
		}

		targetHvacState := computeHvacState(zone.HvacMode, temp, zone.TargetTempC)
		logger.Infof("Average temp: %v, Desired temp: %v, HVAC to %v", temp, zone.TargetTempC, targetHvacState)

		for _, hvac := range zone.Hvacs {
			logger.Infof("Adjusting hvac %v to %v", hvac, targetHvacState)
			err := toggleHVACSwitch(context.Background(), hvac.MachineURI, targetHvacState, logger)
			if err != nil {
				logger.Error("Error toggling hvac. ", hvac)
			}
		}
	}
}

func computeHvacState(hvacMode string, sensorTemp float64, desiredTemp float64) bool {

	if hvacMode == "heat" && sensorTemp > desiredTemp {
		return false
	}

	if hvacMode == "cool" && sensorTemp < desiredTemp {
		return false
	}

	return true
}

func ParseConfig(configbytes []byte, conf *Config) error {
	if err := json.Unmarshal(configbytes, conf); err != nil {
		return err
	}

	return nil
}

func toggleHVACSwitch(ctx context.Context, uri string, on bool, logger *zap.SugaredLogger) error {
	robot, err := client.New(
		context.Background(),
		uri,
		logger,
		client.WithDialOptions(rpc.WithEntityCredentials(
			Conf.HvacAPIName,
			rpc.Credentials{
				Type:    rpc.CredentialsTypeAPIKey,
				Payload: Conf.HvacAPIKey,
			})),
	)
	if err != nil {
		logger.Fatal(err)
	}
	defer robot.Close(ctx)

	switcher, err := generic.FromRobot(robot, "AC-switch-generic")
	if err != nil {
		return err
	}

	command := map[string]interface{}{
		"AC_ON": on,
	}

	_, err = switcher.DoCommand(context.Background(), command)
	return err
}

func getAverageTempFromApp(ctx context.Context, machines []string, logger *zap.SugaredLogger) (float64, error) {

	conn, err := microtemp.AppClient(ctx, Conf.AppAPIName, Conf.AppAPIKey, logger)
	if err != nil {
		return 0, err
	}
	defer (*conn).Close()

	appclient := appd.NewDataServiceClient(*conn)

	matchStage := bson.M{"$match": bson.M{
		"organization_id": Conf.AppOrganizationId,
		"component_name":  "temp",
		"method_name":     "Readings",
		"robot_id":        bson.M{"$in": machines},
		"time_received":   bson.M{"$gt": time.Now().Add(-1 * time.Hour)},
	},
	}
	matchStageBSON, _ := bson.Marshal(matchStage)

	groupStage := bson.M{"$group": bson.M{
		"_id":      nil, //"$robot_id",
		"temp_avg": bson.M{"$avg": "$data.readings.temp"},
		"temp_cnt": bson.M{"$sum": 1},
	}}
	groupStageBSON, _ := bson.Marshal(groupStage)

	request := appd.TabularDataByMQLRequest{
		OrganizationId: Conf.AppOrganizationId,
		MqlBinary:      [][]byte{matchStageBSON, groupStageBSON},
	}

	response, err := appclient.TabularDataByMQL(ctx, &request)
	if err != nil {
		return 0, err
	}

	logger.Info(response)

	if len(response.Data) != 1 {
		return 0, errors.New("wrong number of results")
	}

	datamap := response.Data[0].AsMap()
	if datamap["temp_cnt"].(float64) < 5 { // or some reasonable number
		logger.Errorf("Result based on %v samples", datamap["tmp_cnt"])
		return 0, errors.New("not enough samples")
	}

	return datamap["temp_avg"].(float64), nil
}
