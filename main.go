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

type config struct {
	RobotAPIName string `json:"robot_api_name"`
	RobotAPIKey  string `json:"robot_api_key"`
	AppAPIName   string `json:"app_api_name"`
	AppAPIKey    string `json:"app_api_key"`
}

var Config config

func main() {
	logger := golog.NewDevelopmentLogger("client")
	err := ParseConfig()
	if err != nil {
		logger.Fatal(err)
	}
	logger.Infof("Loaded config file with secrets: %+v", Config)

	for i := 0; i < 6; i++ {

		err = DoAll(logger)
		if err != nil {
			logger.Error(err)
		}

		time.Sleep(SLEEPTIME + 30*time.Second)
	}
}

func DoAll(logger *zap.SugaredLogger) error {
	logger.Info("Connecting to 'smart' machine...")

	robot, err := RobotClient(context.Background(), logger, 5)
	if err != nil {
		return err
	}
	defer robot.Close(context.Background())

	logger.Info("Connected")

	temp, err := ReadTemp(robot, 20, logger)
	if err != nil {
		return err
	}

	logger.Infof("Temp: %v", temp)

	err = SendData(context.Background(), map[string]interface{}{"temp": temp}, logger)
	if err != nil {
		return err
	}

	// will hang
	GoToSleep(robot, 30*time.Second, logger)

	return nil
}

func ParseConfig() error {
	configBytes, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".viam", "temperatureconfig"))
	if err != nil {
		return err
	}

	if err := json.Unmarshal(configBytes, &Config); err != nil {
		return err
	}

	return nil
}

func GoToSleep(robot *client.RobotClient, dur time.Duration, logger *zap.SugaredLogger) error {
	esp, err := board.FromRobot(robot, "b")
	if err != nil {
		return err
	}

	// short b/c SetPowerMode won't return
	ctxShortTime, cancl := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancl()
	err = esp.SetPowerMode(ctxShortTime, rawboard.PowerMode_POWER_MODE_OFFLINE_DEEP, &dur)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		logger.Info("Night night")
		return nil
	}

	return err
}

func ReadTemp(robot *client.RobotClient, numReadings int, logger *zap.SugaredLogger) (float32, error) {
	esp, err := board.FromRobot(robot, "b")
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

func RobotClient(ctx context.Context, logger *zap.SugaredLogger, numRetries int) (*client.RobotClient, error) {
	var err error = nil

	for i := 0; i < numRetries; i++ {
		var robot *client.RobotClient
		ctx, cancelfx := context.WithTimeout(ctx, SLEEPTIME)
		defer cancelfx()

		robot, err = client.New(
			ctx,
			"esp-standalone-main.6xs7zv3bxz.viam.cloud",
			logger,
			client.WithDisableSessions(),
			client.WithReconnectEvery(0),
			client.WithCheckConnectedEvery(0),
			client.WithRefreshEvery(0),
			client.WithDialOptions(rpc.WithEntityCredentials(
				Config.RobotAPIName,
				rpc.Credentials{
					Type:    rpc.CredentialsTypeAPIKey,
					Payload: Config.RobotAPIKey,
				})),
		)

		if err == nil {
			return robot, nil
		}

		logger.Info("Connection to robot failed, sleep and try again.", err)
		time.Sleep(1 * time.Second)
	}

	return nil, err
}

func AppClient(ctx context.Context, logger *zap.SugaredLogger) (*rpc.ClientConn, error) {
	conn, err := rpc.DialDirectGRPC(
		context.Background(),
		"app.viam.com:443",
		logger,
		rpc.WithEntityCredentials(
			Config.AppAPIName,
			rpc.Credentials{
				Type:    rpc.CredentialsTypeAPIKey,
				Payload: Config.AppAPIKey,
			}),
	)

	return &conn, err
}

func SendData(ctx context.Context, values map[string]interface{}, logger *zap.SugaredLogger) error {

	logger.Info("Connecting to app")
	app, err := AppClient(ctx, logger)
	if err != nil {
		return err
	}

	logger.Info("Connected")
	data, _ := structpb.NewStruct(map[string]interface{}{"readings": values})

	logger.Info("Data: ", data)

	request := appds.DataCaptureUploadRequest{
		Metadata: &appds.UploadMetadata{
			PartId:        "3910b942-5228-4e73-ae09-eb668a5ddb1d",
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

	appclient := appds.NewDataSyncServiceClient(*app)
	_, err = appclient.DataCaptureUpload(ctx, &request)

	return err
}
