package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/edaniels/golog"
	"go.uber.org/zap"
	rawboard "go.viam.com/api/component/board/v1"

	appds "go.viam.com/api/app/datasync/v1"

	"go.viam.com/rdk/components/board"
	"go.viam.com/rdk/robot/client"
	"go.viam.com/utils/rpc"
	structpb "google.golang.org/protobuf/types/known/structpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

const SLEEPTIME = 2 * time.Minute
const ITERATIONS = 1

type MachineConfig struct {
	PartId         string `json:"part_id"`
	PartURI        string `json:"part_uri"` // temporary, can be derived
	MachineAPIName string `json:"mach_api_name"`
	MachineAPIKey  string `json:"mach_api_key"`
}

type Config struct {
	Parts      []MachineConfig `json:"machines"`
	AppAPIName string          `json:"app_api_name"`
	AppAPIKey  string          `json:"app_api_key"`
}

var Conf Config
var App *rpc.ClientConn

func main() {
	logger := golog.NewDevelopmentLogger("client")
	err := ParseConfig()
	if err != nil {
		logger.Fatal(err)
	}
	logger.Infof("Loaded config file with secrets: %+v", Conf)

	ctx := context.Background()

	App, err = AppClient(ctx, logger)
	if err != nil {
		logger.Fatal(err)
	}

	for i := 0; i < ITERATIONS; i++ {
		err = DoAll(ctx, Conf.Parts[0], logger)
		if err != nil {
			logger.Error(err)
		}

		// don't sleep on the last iteration
		if i < ITERATIONS-1 {
			logger.Info("Sleeping...")
			time.Sleep(SLEEPTIME + 30*time.Second)
		}
	}
}

func DoAll(ctx context.Context, part MachineConfig, logger *zap.SugaredLogger) error {
	logger.Info("Connecting to 'smart' machine...")

	robot, err := RobotClient(ctx, part, logger, 5)
	if err != nil {
		return err
	}
	defer robot.Close(ctx)

	logger.Info("Connected")

	temp, err := ReadTemp(robot, 20, logger)
	if err != nil {
		return err
	}

	logger.Infof("Temp: %v", temp)

	err = SendData(ctx, part.PartId, map[string]interface{}{"temp": temp}, logger)
	if err != nil {
		return err
	}

	// will hang + timeout. those errors are handled by the function.
	err = GoToSleep(ctx, robot, SLEEPTIME, logger)
	if err != nil {
		return err
	}

	return nil
}

func ParseConfig() error {
	configBytes, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".viam", "temperatureconfig"))
	if err != nil {
		return err
	}

	if err := json.Unmarshal(configBytes, &Conf); err != nil {
		return err
	}

	return nil
}

func GoToSleep(ctx context.Context, robot *client.RobotClient, dur time.Duration, logger *zap.SugaredLogger) error {
	esp, err := board.FromRobot(robot, "board")
	if err != nil {
		return err
	}

	// short b/c SetPowerMode won't return
	ctxShortTime, cancl := context.WithTimeout(ctx, 2*time.Second)
	defer cancl()
	err = esp.SetPowerMode(ctxShortTime, rawboard.PowerMode_POWER_MODE_OFFLINE_DEEP, &dur)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		logger.Info("Night night")
		return nil
	}

	return err
}

func ReadTemp(robot *client.RobotClient, numReadings int, logger *zap.SugaredLogger) (float32, error) {
	esp, err := board.FromRobot(robot, "board")
	if err != nil {
		return 0, err
	}

	analog, exists := esp.AnalogReaderByName("temp")
	if !exists {
		return 0, errors.New("no analog reader 'temp' found")
	}

	var sum float32
	var numRealReadings int
	for i := 0; i < numReadings; i++ {
		reading, err := analog.Read(context.Background(), nil)
		if err != nil {
			logger.Info("Failed to get reading, skipping.", err)
			continue
		}

		t := float32(reading-500) / 10.0
		logger.Debugf("%v: temp: %f", i, t)
		sum += t
		numRealReadings++
		time.Sleep(10 * time.Millisecond)
	}
	// proxy for no readings
	if sum == 0 {
		return 0, errors.New("no temp readings received")
	}

	temp := sum / float32(numRealReadings)
	return temp, nil
}

func RobotClient(ctx context.Context, part MachineConfig, logger *zap.SugaredLogger, numRetries int) (*client.RobotClient, error) {
	var err error = nil

	for i := 0; i < numRetries; i++ {
		var robot *client.RobotClient
		ctx, cancelfx := context.WithTimeout(ctx, SLEEPTIME)
		defer cancelfx()

		robot, err = client.New(
			ctx,
			part.PartURI,
			logger,
			client.WithDisableSessions(),
			client.WithReconnectEvery(0),
			client.WithCheckConnectedEvery(0),
			client.WithRefreshEvery(0),
			client.WithDialOptions(rpc.WithEntityCredentials(
				part.MachineAPIName,
				rpc.Credentials{
					Type:    rpc.CredentialsTypeAPIKey,
					Payload: part.MachineAPIKey,
				})),
		)

		if err == nil {
			return robot, nil
		}

		logger.Info("Connection to machine failed, sleep and try again.", err)
		time.Sleep(1 * time.Second)
	}

	return nil, err
}

func AppClient(ctx context.Context, logger *zap.SugaredLogger) (*rpc.ClientConn, error) {
	logger.Info("Connecting to app")

	conn, err := rpc.DialDirectGRPC(
		context.Background(),
		"app.viam.com:443",
		logger,
		rpc.WithEntityCredentials(
			Conf.AppAPIName,
			rpc.Credentials{
				Type:    rpc.CredentialsTypeAPIKey,
				Payload: Conf.AppAPIKey,
			}),
	)
	logger.Info("Connected")

	return &conn, err
}

func SendData(ctx context.Context, part_id string, values map[string]interface{}, logger *zap.SugaredLogger) error {
	data, _ := structpb.NewStruct(map[string]interface{}{"readings": values})
	logger.Info("Sending data: ", data)

	request := appds.DataCaptureUploadRequest{
		Metadata: &appds.UploadMetadata{
			PartId:        part_id,
			ComponentType: "rdk:component:sensor",
			ComponentName: "temp",
			MethodName:    "Readings",
			Type:          appds.DataType_DATA_TYPE_TABULAR_SENSOR,
		},
		SensorContents: []*appds.SensorData{
			{
				Metadata: &appds.SensorMetadata{
					TimeRequested: timestamppb.Now(),
					TimeReceived:  timestamppb.Now(),
				},
				Data: &appds.SensorData_Struct{Struct: data},
			}},
	}

	appclient := appds.NewDataSyncServiceClient(*App)
	_, err := appclient.DataCaptureUpload(ctx, &request)

	return err
}
