/***********************************************************************************
PROJECT NAME  Mqtt Client...
DESCRIPTION   Mqtt Client封包處埋.....
HardWare      ***
Copyright   : 2018 TECO Ltd.
All Rights Reserved
//influxdb使用...
influx
> create user teco with password 'teco1133'
> grant all privileges to teco  //授權全部的資料庫(等於管理者帳號)...
> CREATE DATABASE chiler
> USE chiler
> SELECT * FROM "MAC_013157800087586"


***********************************************************************************/
/***********************************************************************************
Revision History
DD.MM.YYYY OSO-UID Description..
30.08.2018 Thomas start work..
25.09.2018 Thomas 修改成NB-IoT方案,import github用指令 go get 加入libery...
16.10.2018 Thomas 修改收NB-IoT,打到influx...
21.10.2018 Thomas 本版可以針對特定的MAC,把Data上傳到AWS...
22.10.2018 Thomas 修改判斷是否為03設備(空氣偵測),再上傳AWS...
06.11.2018 Thomas 把上傳次數暫放在電量欄位...
15.01.2019 Thomas 增加MQTT 自動check功能...
19.01.2019 Thomas 修改東元冰箱溫度顯示機能....
19.03.2019 Thomas 增冰水機passer,MQTT主機...35.201.226.72...
02.04.2019 Thomas 增Mongodb功能...
11.05.2019 Thomas 新增冰水機左機及右機及PLC時間資料...
***********************************************************************************/
package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"time"
	//import the Paho Go MQTT library
	MQTT "github.com/eclipse/paho.mqtt.golang"
	mgo "gopkg.in/mgo.v2"
)

type ChilerDB struct {
	CoolWaterIn      string
	CoolWaterOut     string
	IceWaterIn       string
	IceWaterOut      string
	PipeOutTemp_L    string
	PipeOutTemp_R    string
	CompHiPress_L    string
	CompLowPress_L   string
	CompHiPress_R    string
	CompLowPress_R   string
	Voltage          string
	Circuit_L        string
	Circuit_R        string
	LEV_L            string
	LEV_R            string
	CompFrequency    string
	tscTemp_L        string
	TseTemp_L        string
	TscTemp_R        string
	TseTemp_R        string
	KW_L             string
	KW_R             string
	SetTemperature   string
	LeftCompRunTime  string
	RightCompRunTime string
	SystemMinute     string
	TimeStamp        string
}

//
const (
	MyDB     = "chiler"
	username = "teco"
	password = "teco1133"

//	MyMeasurement = "cpu_usage"
)

//設定MQTT接收的Buffer...
const (
	MaxClientIdLen = 10
)

var Knt int
var awsKnt int
var rxdata [300]uint8
var txdata [148]rune
var rxMac [16]uint8
var awsTxData [22]uint8
var water_temp_in [4]uint8

var powerMeter uint16
var powerMeterH uint16
var powerMeterL uint16
var powerMetertemp uint8
var powerMetertemp16 uint16
var powerIndex uint16

var testData float32
var data [13]float32

var roomTemperAve uint8    //二個門平均溫度...
var topRoomTemper uint8    //上門溫度...
var minRoomTemper uint8    //中門溫度...
var bottomRoomTemper uint8 //下門溫度...

//Chiler 變數...
var awsUpdataTemp uint16 //上傳AWS專用...
var collWaterIn float32
var collWaterOut float32
var iceWaterIn float32
var iceWaterOut float32
var pipeOutTemp_L float32
var pipeOutTemp_R float32
var compHiPress_L float32
var compLowPress_L float32
var compHiPress_R float32
var compLowPress_R float32
var voltage float32
var circuit_L float32
var circuit_R float32
var lEV_L float32
var lEV_R float32
var compFrequency float32
var tscTemp_L float32
var tseTemp_L float32
var tscTemp_R float32
var tseTemp_R float32
var kW_L float32
var kW_R float32
var setTemperature float32
var LeftCompRunTime float32
var RightCompRunTime float32
var SystemMinute float32

func StringToRuneArr(s string, arr []rune) {
	src := []rune(s)
	for i, v := range src {
		if i >= len(arr) {
			break
		}
		arr[i] = v
	}
}

//處理數值轉換...
func ChangeData(s_1 uint8, s_2 uint8) (dataTemp uint8) {
	if s_1 >= '0' && s_1 <= '9' {
		dataTemp = (s_1 - 0x30) << 4
	}
	if s_1 >= 'a' && s_1 <= 'f' {
		dataTemp = (s_1 - 0x61 + 10) << 4
	}
	if s_1 >= 'A' && s_1 <= 'F' {
		dataTemp = (s_1 - 0x41 + 10) << 4
	}

	if s_2 >= '0' && s_2 <= '9' {
		dataTemp = dataTemp + (s_2 - 0x30)
	}
	if s_2 >= 'a' && s_2 <= 'f' {
		dataTemp = dataTemp + (s_2 - 0x61 + 10)
	}
	if s_2 >= 'A' && s_2 <= 'F' {
		dataTemp = dataTemp + (s_2 - 0x41 + 10)
	}

	return
}

//處理上傳IAQ的 10位字元
func Send2IAQ_10(dataIn uint16) (dataOut uint8) {
	var dataTemp uint8
	dataTemp = uint8(dataIn) & 0xf0
	dataTemp = dataTemp >> 4

	if dataTemp >= 0 && dataTemp <= 9 {
		dataOut = 0x30 + dataTemp
	} else if dataTemp >= 10 && dataTemp <= 15 {
		dataTemp = dataTemp - 10
		dataOut = 0x61 + dataTemp
	}
	return
}

//處理上傳IAQ的 個位字元
func Send2IAQ_1(dataIn uint16) (dataOut uint8) {
	var dataTemp uint8
	dataTemp = uint8(dataIn) & 0x0f

	if dataTemp >= 0 && dataTemp <= 9 {
		dataOut = 0x30 + dataTemp
	} else if dataTemp >= 10 && dataTemp <= 15 {
		dataTemp = dataTemp - 10
		dataOut = 0x61 + dataTemp
	}
	return
}

// var outgoing chan *MQTTMessage

//define a function for the default message handler
//set callback function
var f MQTT.MessageHandler = func(client MQTT.Client, msg MQTT.Message) {
	//	mQTTMessage := &MQTTMessage{msg, m}
	//	fmt.Printf("TOPIC: %s\n", msg.Topic())
	//txdata[0] = "{\"address\":\"0013157800087578\",\"data\":\"1010001a19950000000001\",\"time\":\"2018-10-21 23:34:52\",\"gwid\":\"00001c497bcaafea\",\"rssi\":-77,\"channel\":922625000}"

	var indexMessage int
	Knt++
	if Knt > 65530 {
		Knt = 0
	}
	fmt.Printf("MSG: %d\n", Knt)
	//fmt.Printf("%s\n", msg.Payload())

	//轉換資料...
	s := string(msg.Payload()[:])
	//	fmt.Printf("%s\n", s)
	//fmt.Println(strings.Contains(s, "data"))
	//比較字串,並接收資料...
	//if strings.Contains(s, "data\":\"0311") {
	//if strings.Contains(s, "013157800087560") {
	if strings.Contains(s, "data") {
		indexMessage = strings.Index(s, "mac")
		/*
			//MAC=7878
			if strings.Contains(s, "013157800087578") {
				rxMac[0] = '0'
				fmt.Printf("index %d\n", indexMessage)
				for i := 0; i < 15; i++ {
					rxMac[i+1] = s[indexMessage+i+6]
				}
			} else { //不是7578...
				fmt.Printf("index %d\n", indexMessage)
				for i := 0; i < 16; i++ {
					rxMac[i] = s[indexMessage+i+6]
				}
			}
		*/

		fmt.Printf("index %d\n", indexMessage)
		for i := 0; i < 16; i++ {
			rxMac[i] = s[indexMessage+i+6]
		}
		//如果收到fff,不處理...
		if strings.Contains(s, "ffff") {
			fmt.Printf("Receive [ffff] data from NB-Iot %s\n", rxMac)
			return
		}
		fmt.Printf("MAC: %s\n", rxMac)
		indexMessage = strings.Index(s, "data")
		fmt.Printf("index %d\n", indexMessage)
		//for i := 0; i < 58; i++ {
		mqttMessageLength := len([]rune(s))
		fmt.Printf("messagelength: %d\n", mqttMessageLength)
		for i := 0; i < mqttMessageLength-36; i++ { //原108...
			rxdata[i] = s[indexMessage+i+7]
		}

		fmt.Printf("Rxdata: %s\n", rxdata)

		//寫入influxdb...

		//建立第二個MQTT Cliend...
		awsTopic := "iaq"
		clientId := getRandomClientId()
		fmt.Printf("clientId: %s\n", clientId)
		awsMQTTBroker := MQTT.NewClientOptions().AddBroker("tcp://13.114.3.126:1883")
		awsMQTTBroker.SetClientID(clientId)
		awsMQTTBroker.SetDefaultPublishHandler(awsfun)

		//create and start a client using the above ClientOptions
		awsClient := MQTT.NewClient(awsMQTTBroker)
		/*
			if awsToken := awsClient.Connect(); awsToken.Wait() && awsToken.Error() != nil {
				//panic(awsToken.Error())
				log.Fatal(awsToken.Error())
			} else {
				fmt.Printf("Connected to 13.114.3.126 server\n")
			}
		*/
		awsToken := awsClient.Connect()
		for !awsToken.WaitTimeout(3 * time.Second) {
			fmt.Printf("Wait time out connected to 13.114.3.126 server\n")
		}
		if err := awsToken.Error(); err != nil {
			log.Fatal(err)
			fmt.Printf("No connected to 13.114.3.126 server\n")
			awsClient.Unsubscribe(awsTopic)
			if awsEndToken := awsClient.Unsubscribe(awsTopic); awsEndToken.Wait() && awsEndToken.Error() != nil {
				fmt.Println(awsEndToken.Error())
				fmt.Printf("awsEndToken Error 13.114.3.126 server\n")
			}

		} else {

			fmt.Printf("Connected to 13.114.3.126 server\n")

		}

		//設定時間格式
		t := time.Now()
		//var timestamp int64 = 1498003200
		//fmt.Println(t.UTC().Format(time.UnixDate))
		//ok
		//3 and 15 for hour PM...
		//2016 for year
		getTimer := t.Format("2006-01-02 15:04:05")
		fmt.Println(getTimer)

		timestamp := strconv.FormatInt(t.UTC().UnixNano(), 10)
		//timestamp := time.Now()
		//fmt.Println(timestamp)
		timestamp = timestamp[:10]
		i, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			panic(err)
		}
		tm := time.Unix(i, 0)
		//fmt.Println(tm)
		//tm2 := time.Unix(timestamp, 0)
		fmt.Println(tm.Format("2006-01-02 03:04:05"))

		/*
			if awsToken := awsClient.Subscribe(awsTopic, 0, nil); awsToken.Wait() && awsToken.Error() != nil {
				fmt.Println(awsToken.Error())
				awsToken.Wait()
				//	os.Exit(1)
			}
		*/
		//字串處理
		awsstr := "{\"address\":\"MMMMMMMMMMMMMMMM\",\"data\":\"######################\",\"time\":\"*******************\",\"gwid\":\"00001c497bcaafea\",\"rssi\":-77,\"channel\":922625000}"
		awsstr = strings.Replace(awsstr, "MMMMMMMMMMMMMMMM", string(rxMac[:]), -1)
		awsstr = strings.Replace(awsstr, "*******************", getTimer, -1)
		/*
			//原室內機...
			if rxdata[0] == '0' && rxdata[1] == '1' {
				//awsstr = strings.Replace(awsstr, "######################", "1010001a19950000000001", -1)
				awsTxData[0] = '1'
				awsTxData[1] = '0'
				//power...
				awsTxData[2] = '1'
				//mode...
				awsTxData[3] = '0'
				//fan speed...
				awsTxData[4] = '0'
				//fan drict...
				awsTxData[5] = '0'
				//Room temperature setting...
				awsTxData[6] = rxdata[56]
				awsTxData[7] = rxdata[57]
				//Room temperature...
				awsTxData[8] = rxdata[58]
				awsTxData[9] = rxdata[59]
				//power consumption
				awsTxData[10] = '0'
				awsTxData[11] = '0'
				//error code...
				awsTxData[12] = '0'
				awsTxData[13] = '0'
				awsTxData[14] = '0'
				awsTxData[15] = '0'
				awsTxData[16] = '0'
				awsTxData[17] = '0'
				awsTxData[18] = '0'
				awsTxData[19] = '0'
				awsTxData[20] = '0'
				awsTxData[21] = '0'
				fmt.Printf("water_temp_in: %s\n", string(water_temp_in[0:4]))
				awsstr = strings.Replace(awsstr, "######################", string(awsTxData[:]), -1)
			}
		*/

		if rxdata[0] == '0' && rxdata[1] == '1' {
			awsTxData[0] = '7'
			awsTxData[1] = '0'
			//1.冷卻水出口溫度...//4,5,6,7...
			//存mongo..
			collWaterOut = float32(uint16(ChangeData(rxdata[4], rxdata[5])) << 8)
			collWaterOut = collWaterOut + float32(ChangeData(rxdata[6], rxdata[7]))
			fmt.Printf("collWaterOut: %d\n", collWaterOut)
			collWaterOut = collWaterOut / 10
			//上傳aws...
			awsUpdataTemp = uint16(ChangeData(rxdata[4], rxdata[5])) << 8
			awsUpdataTemp = awsUpdataTemp + uint16(ChangeData(rxdata[6], rxdata[7]))
			fmt.Printf("冰水出口: %d\n", awsUpdataTemp)
			awsUpdataTemp = awsUpdataTemp / 10
			fmt.Printf("Send2IAQ_10(CollWaterOut): %d\n", awsUpdataTemp)
			awsTxData[2] = Send2IAQ_10(awsUpdataTemp)
			awsTxData[3] = Send2IAQ_1(awsUpdataTemp)
			//2.冷卻水入口溫度...//8,9,10,11...
			//mongo...
			collWaterIn = float32(uint16(ChangeData(rxdata[8], rxdata[9])) << 8)
			collWaterIn = collWaterIn + float32(ChangeData(rxdata[10], rxdata[11]))
			collWaterIn = collWaterIn / 10
			//aws...
			awsUpdataTemp = uint16(ChangeData(rxdata[8], rxdata[9])) << 8
			awsUpdataTemp = awsUpdataTemp + uint16(ChangeData(rxdata[10], rxdata[11]))
			awsUpdataTemp = awsUpdataTemp / 10
			awsTxData[4] = Send2IAQ_10(awsUpdataTemp) //10
			awsTxData[5] = Send2IAQ_1(awsUpdataTemp)  //11
			//3.冰水出口溫度...//12,13,14,15..
			//monfg...
			iceWaterOut = float32(uint16(ChangeData(rxdata[12], rxdata[13])) << 8)
			iceWaterOut = iceWaterOut + float32(ChangeData(rxdata[14], rxdata[15]))
			iceWaterOut = iceWaterOut / 10
			//aws...
			awsUpdataTemp = uint16(ChangeData(rxdata[12], rxdata[13])) << 8
			awsUpdataTemp = awsUpdataTemp + uint16(ChangeData(rxdata[14], rxdata[15]))
			awsUpdataTemp = awsUpdataTemp / 10
			awsTxData[6] = Send2IAQ_10(awsUpdataTemp) //rxdata[14]
			awsTxData[7] = Send2IAQ_1(awsUpdataTemp)  //rxdata[15]
			//4.冰水入口溫度...	//16,17,18,19..
			//mongo
			iceWaterIn = float32(uint16(ChangeData(rxdata[16], rxdata[17])) << 8)
			iceWaterIn = iceWaterIn + float32(ChangeData(rxdata[18], rxdata[19]))
			iceWaterIn = iceWaterIn / 10
			//aws
			awsUpdataTemp = uint16(ChangeData(rxdata[16], rxdata[17])) << 8
			awsUpdataTemp = awsUpdataTemp + uint16(ChangeData(rxdata[18], rxdata[19]))
			awsUpdataTemp = awsUpdataTemp / 10
			awsTxData[8] = Send2IAQ_10(awsUpdataTemp) //rxdata[18]
			awsTxData[9] = Send2IAQ_1(awsUpdataTemp)  //rxdata[19]
			//5.左機吐出溫度...			//20,21,22,23..
			//mogo
			pipeOutTemp_L = float32(uint16(ChangeData(rxdata[20], rxdata[21])) << 8)
			pipeOutTemp_L = pipeOutTemp_L + float32(ChangeData(rxdata[22], rxdata[23]))
			pipeOutTemp_L = pipeOutTemp_L / 10
			//aws..
			awsUpdataTemp = uint16(ChangeData(rxdata[20], rxdata[21])) << 8
			awsUpdataTemp = awsUpdataTemp + uint16(ChangeData(rxdata[22], rxdata[23]))
			awsUpdataTemp = awsUpdataTemp / 10
			awsTxData[10] = Send2IAQ_10(awsUpdataTemp) //rxdata[22]
			awsTxData[11] = Send2IAQ_1(awsUpdataTemp)  //rxdata[23]
			//6.右機吐出溫度...	//24,25,26.27..
			//mongo..
			pipeOutTemp_R = float32(uint16(ChangeData(rxdata[24], rxdata[25])) << 8)
			pipeOutTemp_R = pipeOutTemp_R + float32(ChangeData(rxdata[26], rxdata[27]))
			pipeOutTemp_R = pipeOutTemp_R / 10
			//aws..
			awsUpdataTemp = uint16(ChangeData(rxdata[24], rxdata[25])) << 8
			awsUpdataTemp = awsUpdataTemp + uint16(ChangeData(rxdata[26], rxdata[27]))
			awsUpdataTemp = awsUpdataTemp / 10
			awsTxData[12] = Send2IAQ_10(awsUpdataTemp) //rxdata[26]
			awsTxData[13] = Send2IAQ_1(awsUpdataTemp)  //rxdata[27]
			//7.左機高壓...	//28,29,30,31..
			//mongo
			compHiPress_L = float32(uint16(ChangeData(rxdata[28], rxdata[29])) << 8)
			compHiPress_L = compHiPress_L + float32(ChangeData(rxdata[30], rxdata[31]))
			fmt.Printf("Send2IAQ_10(compHiPress_L): %d\n", compHiPress_L)
			compHiPress_L = compHiPress_L / 100
			//aws...
			awsUpdataTemp = uint16(ChangeData(rxdata[28], rxdata[29])) << 8
			awsUpdataTemp = awsUpdataTemp + uint16(ChangeData(rxdata[30], rxdata[31]))
			fmt.Printf("Send2IAQ_10(compHiPress_L): %d\n", awsUpdataTemp)
			awsUpdataTemp = awsUpdataTemp / 100
			fmt.Printf("Send2IAQ_10(compHiPress_L/100): %d\n", awsUpdataTemp)
			awsTxData[14] = Send2IAQ_10(awsUpdataTemp * 10) //rxdata[30]
			awsTxData[15] = Send2IAQ_1(awsUpdataTemp * 10)  //rxdata[31]
			//8.左機低壓...//32,33,34,35..
			//mongo
			compLowPress_L = float32(uint16(ChangeData(rxdata[32], rxdata[33])) << 8)
			compLowPress_L = compLowPress_L + float32(ChangeData(rxdata[34], rxdata[35]))
			fmt.Printf("Send2IAQ_10(compLowPress_L): %d\n", compLowPress_L)
			compLowPress_L = compLowPress_L / 100
			//9.右機高壓...			//36,37,38,39
			//mongo
			compHiPress_R = float32(uint16(ChangeData(rxdata[36], rxdata[37])) << 8)
			compHiPress_R = compHiPress_R + float32(ChangeData(rxdata[38], rxdata[39]))
			compHiPress_R = compHiPress_R / 100
			//aws...
			awsUpdataTemp = uint16(ChangeData(rxdata[36], rxdata[37])) << 8
			awsUpdataTemp = awsUpdataTemp + uint16(ChangeData(rxdata[38], rxdata[39]))
			awsUpdataTemp = awsUpdataTemp / 100
			awsTxData[16] = Send2IAQ_10(awsUpdataTemp * 10) //rxdata[38]
			awsTxData[17] = Send2IAQ_1(awsUpdataTemp * 10)  //rxdata[39]
			//10.右機低壓...	//40,41,42,43..
			//mongo
			compLowPress_R = float32(uint16(ChangeData(rxdata[40], rxdata[41])) << 8)
			compLowPress_R = compLowPress_R + float32(ChangeData(rxdata[42], rxdata[43]))
			compLowPress_R = compLowPress_R / 100
			//11.電壓...	//44,45,46,47...
			//mongo...
			voltage = float32(uint16(ChangeData(rxdata[44], rxdata[45])) << 8)
			fmt.Printf("voltage: %g\n", voltage)
			voltage = voltage + float32(ChangeData(rxdata[46], rxdata[47]))
			fmt.Printf("voltage: %g\n", voltage)
			voltage = voltage / 10
			fmt.Printf("rxdata[44]: %c\n", rxdata[44])
			fmt.Printf("rxdata[45]: %c\n", rxdata[45])
			fmt.Printf("rxdata[46]: %c\n", rxdata[46])
			fmt.Printf("rxdata[47]: %c\n", rxdata[47])
			fmt.Printf("voltage: %g\n", voltage)
			//12.左機電流...//48,49,50,51..
			//mongo...
			circuit_L = float32(uint16(ChangeData(rxdata[48], rxdata[49])) << 8)
			circuit_L = circuit_L + float32(ChangeData(rxdata[50], rxdata[51]))
			circuit_L = circuit_L / 10
			//aws..
			awsUpdataTemp = uint16(ChangeData(rxdata[48], rxdata[49])) << 8
			awsUpdataTemp = awsUpdataTemp + uint16(ChangeData(rxdata[50], rxdata[51]))
			awsUpdataTemp = awsUpdataTemp / 10
			awsTxData[18] = Send2IAQ_10(awsUpdataTemp) //rxdata[50]
			awsTxData[19] = Send2IAQ_1(awsUpdataTemp)  //rxdata[51]
			//13.右機電流...//52,53,54,55...
			//mongo..
			circuit_R = float32(uint16(ChangeData(rxdata[52], rxdata[53])) << 8)
			circuit_R = circuit_R + float32(ChangeData(rxdata[54], rxdata[55]))
			circuit_R = circuit_R / 10
			//aws...
			awsUpdataTemp = uint16(ChangeData(rxdata[52], rxdata[53])) << 8
			awsUpdataTemp = awsUpdataTemp + uint16(ChangeData(rxdata[54], rxdata[55]))
			awsUpdataTemp = awsUpdataTemp / 10
			awsTxData[20] = Send2IAQ_10(awsUpdataTemp) //rxdata[54]
			awsTxData[21] = Send2IAQ_1(awsUpdataTemp)  //rxdata[55]
			//14.左機LEV開度...	//56,57,58,59...
			//momgo..
			lEV_L = float32(uint16(ChangeData(rxdata[56], rxdata[57])) << 8)
			lEV_L = lEV_L + float32(ChangeData(rxdata[58], rxdata[59]))
			//15.右機LEV開度...	//60,61,62,63,momgo...
			lEV_R = float32(uint16(ChangeData(rxdata[60], rxdata[61])) << 8)
			lEV_R = lEV_R + float32(ChangeData(rxdata[62], rxdata[63]))
			//16.壓縮機頻率...	//64,65,66,67,mongo...
			compFrequency = float32(uint16(ChangeData(rxdata[64], rxdata[65])) << 8)
			compFrequency = compFrequency + float32(ChangeData(rxdata[66], rxdata[67]))
			//17.左飽和冷凝溫度(Tsc)...	//68,69,70,71,mongo..
			tscTemp_L = float32(uint16(ChangeData(rxdata[68], rxdata[69])) << 8)
			tscTemp_L = tscTemp_L + float32(ChangeData(rxdata[70], rxdata[71]))
			tscTemp_L = tscTemp_L / 10
			//18.左飽和蒸發溫度(Tse)...//72,73,74.75,mongo...
			tseTemp_L = float32(uint16(ChangeData(rxdata[72], rxdata[73])) << 8)
			tseTemp_L = tseTemp_L + float32(ChangeData(rxdata[74], rxdata[75]))
			tseTemp_L = tseTemp_L / 10
			//19.右飽和冷凝溫度(Tsc)...	//76,77,78,79,mongo...
			tscTemp_R = float32(uint16(ChangeData(rxdata[76], rxdata[77])) << 8)
			tscTemp_R = tscTemp_R + float32(ChangeData(rxdata[78], rxdata[79]))
			tscTemp_R = tscTemp_R / 10
			//20.右飽和蒸發溫度(Tse)...		//80,81,82,83,mongo...
			tseTemp_R = float32(uint16(ChangeData(rxdata[80], rxdata[81])) << 8)
			tseTemp_R = tseTemp_R + float32(ChangeData(rxdata[82], rxdata[83]))
			tseTemp_R = tseTemp_R / 10
			//21.左耗電(KW)...		//84,85,86,87,mongo...
			kW_L = float32(uint16(ChangeData(rxdata[84], rxdata[85])) << 8)
			kW_L = kW_L + float32(ChangeData(rxdata[86], rxdata[87]))
			kW_L = kW_L / 10
			//22.右耗電(KW)...	//88,89,90,91,mongo...
			kW_R = float32(uint16(ChangeData(rxdata[88], rxdata[89])) << 8)
			kW_R = kW_R + float32(ChangeData(rxdata[90], rxdata[91]))
			kW_R = kW_R / 10
			//23.設溫...//92,93,94,95,mongo...
			setTemperature = float32(uint16(ChangeData(rxdata[92], rxdata[93])) << 8)
			setTemperature = setTemperature + float32(ChangeData(rxdata[94], rxdata[95]))
			setTemperature = setTemperature / 10
			//24.左機累計運轉時數...//96,97,98,99,mongo...
			LeftCompRunTime = float32(uint16(ChangeData(rxdata[96], rxdata[97])) << 8)
			LeftCompRunTime = LeftCompRunTime + float32(ChangeData(rxdata[98], rxdata[99]))
			LeftCompRunTime = LeftCompRunTime
			//25.右機累計運轉時數...//100,101,102,103,mongo...
			RightCompRunTime = float32(uint16(ChangeData(rxdata[100], rxdata[101])) << 8)
			RightCompRunTime = RightCompRunTime + float32(ChangeData(rxdata[102], rxdata[103]))
			RightCompRunTime = RightCompRunTime
			//26.PLC時間(分)...//104,105,106,107,mongo...
			SystemMinute = float32(uint16(ChangeData(rxdata[104], rxdata[105])) << 8)
			SystemMinute = SystemMinute + float32(ChangeData(rxdata[106], rxdata[107]))
			SystemMinute = SystemMinute

			fmt.Printf("water_temp_in: %s\n", string(water_temp_in[0:4]))
			awsstr = strings.Replace(awsstr, "######################", string(awsTxData[:]), -1)

			//冰水主木機passer#########################################################...
			//建立DB----------------------------------------...
			//mongodb建立...
			session, err := mgo.Dial("127.0.0.1:27017")
			if err != nil {
				panic(err)
			}
			defer session.Close() //用完記得關閉
			// Optional. Switch the session to a monotonic behavior.
			session.SetMode(mgo.Monotonic, true) //讀模式，與複本集有關，詳情參考https://docs.mongodb.com/manual/reference/read-preference/ & https://docs.mongodb.com/manual/replication/
			//string(rxMac[:])
			//db := session.DB("chiller0001").C("0013157800087578") //同時建立DB名子及collection,三重兆豐 ...
			//	db := session.DB("chiller0001").C(string(rxMac[:])) //同時建立DB名子及collection,三重兆豐 ...
			//mongodbstr := "chiler******"
			//轉成字串...
			//strYesr := fmt.Sprintf("%d", t.Year())
			//fmt.Printf("year: \n", strYesr)
			//if(t.Month() < 10)
			//fmt.Printf("Month: \n", strMonth)
			//字串相加,建立Collection,...
			var serialstr string
			strMonth := fmt.Sprintf("%d", t.Month())
			if t.Month() > 9 {
				serialstr = string(rxMac[:]) + "_" + strMonth
			} else {
				serialstr = string(rxMac[:]) + "_0" + strMonth
			}

			//	mongodbstr = strings.Replace(mongodbstr, "******", serialstr, -1)
			fmt.Printf("%s \n", serialstr)
			db := session.DB("chiller2019").C(serialstr) //同時建立DB名子及collection,三重兆豐 ...
			//	db := session.DB(mongodbstr).C(string(rxMac[:])) //同時建立DB名子及collection,三重兆豐 ...
			coolwaterinstr := fmt.Sprintf("%g", collWaterIn)
			coolwateroutstr := fmt.Sprintf("%g", collWaterOut)
			iceWaterInstr := fmt.Sprintf("%g", iceWaterIn)
			iceWaterOutstr := fmt.Sprintf("%g", iceWaterOut)
			pipeOutTemp_Lstr := fmt.Sprintf("%g", pipeOutTemp_L)
			pipeOutTemp_Rstr := fmt.Sprintf("%g", pipeOutTemp_R)
			compHiPress_Lstr := fmt.Sprintf("%g", compHiPress_L)
			compLowPress_Lstr := fmt.Sprintf("%g", compLowPress_L)
			compHiPress_Rstr := fmt.Sprintf("%g", compHiPress_R)
			compLowPress_Rstr := fmt.Sprintf("%g", compLowPress_R)
			voltagestr := fmt.Sprintf("%g", voltage)
			circuit_Lstr := fmt.Sprintf("%g", circuit_L)
			circuit_Rstr := fmt.Sprintf("%g", circuit_R)
			lEV_Lstr := fmt.Sprintf("%g", lEV_L)
			lEV_Rstr := fmt.Sprintf("%g", lEV_R)
			compFrequencystr := fmt.Sprintf("%g", compFrequency)
			tscTemp_Lstr := fmt.Sprintf("%g", tscTemp_L)
			tseTemp_Lstr := fmt.Sprintf("%g", tseTemp_L)
			tscTemp_Rstr := fmt.Sprintf("%g", tscTemp_R)
			tseTemp_Rstr := fmt.Sprintf("%g", tseTemp_R)
			kW_Lstr := fmt.Sprintf("%g", kW_L)
			kW_Rstr := fmt.Sprintf("%g", kW_R)
			setTemperaturestr := fmt.Sprintf("%g", setTemperature)
			LeftCompRunTimestr := fmt.Sprintf("%g", LeftCompRunTime)
			RightCompRunTimestr := fmt.Sprintf("%g", RightCompRunTime)
			SystemMinutestr := fmt.Sprintf("%g", SystemMinute)

			err = db.Insert(&ChilerDB{coolwaterinstr, coolwateroutstr, iceWaterInstr, iceWaterOutstr,
				pipeOutTemp_Lstr, pipeOutTemp_Rstr, compHiPress_Lstr, compLowPress_Lstr, compHiPress_Rstr,
				compLowPress_Rstr, voltagestr, circuit_Lstr, circuit_Rstr, lEV_Lstr, lEV_Rstr, compFrequencystr,
				tscTemp_Lstr, tseTemp_Lstr, tscTemp_Rstr, tseTemp_Rstr, kW_Lstr, kW_Rstr, setTemperaturestr,
				LeftCompRunTimestr, RightCompRunTimestr, SystemMinutestr,
				tm.Format("2006-01-02 15:04:05")}) //輸入資料...
			//err = db.Insert(&ChilerDB{"35", "中文", tm.Format("2006-01-02 03:04:05")}) //輸入資料...
			//err = db.Insert(&ChilerDB{"35", "中文", "2019-03-31 23:13"}, //輸入資料...
			//	&ChilerDB{"26", "30", "2019-02-11 22:14"})

			if err != nil {
				log.Fatal(err)
			}

			result := ChilerDB{}
			//查詢時要用全小寫...
			//	err = db.Find(bson.M{"timestamp": "2019-03-31 23:13"}).One(&result) //如果查詢失敗，返回“not found”
			/*
				err = db.Find(bson.M{"timestamp": "2019-03-31 23:13"}).One(&result) //如果查詢失敗，返回“not found”
				if err != nil {
					log.Fatal(err)
				}
			*/
			fmt.Println("CoolWaterIn:", result.CoolWaterIn, result.CoolWaterOut, result.TimeStamp)

			//mongodb end--------------------------------------

		}
		//冰水主機end #############################################################################################...
		//商用冰箱測試用....
		if rxdata[0] == '0' && rxdata[1] == '6' {

			//awsstr = strings.Replace(awsstr, "######################", "1010001a19950000000001", -1)

			awsTxData[0] = '6'
			awsTxData[1] = '0'
			//power...
			awsTxData[2] = '1'

			//上門溫度...
			awsTxData[3] = rxdata[12]
			awsTxData[4] = rxdata[13]

			//awsTxData[3] = '0'
			//awsTxData[4] = '3'
			//中門溫度...
			awsTxData[5] = rxdata[14]
			awsTxData[6] = rxdata[15]
			//下門溫度
			//awsTxData[7] = rxdata[16]
			//awsTxData[8] = rxdata[17]
			awsTxData[7] = 'k'
			awsTxData[8] = 'k'

			//針對東元變頻,修改溫度...
			if rxMac[12] == '7' && rxMac[13] == '6' && rxMac[14] == '2' && rxMac[15] == '8' {
				//上門溫度...
				if rxdata[12] >= '0' && rxdata[12] <= '9' {
					topRoomTemper = (rxdata[12] - 0x30) * 10
				}
				if rxdata[12] >= 'a' && rxdata[12] <= 'f' {
					topRoomTemper = (rxdata[12] - 0x61 + 10) * 10
				}
				if rxdata[12] >= 'A' && rxdata[12] <= 'F' {
					topRoomTemper = (rxdata[12] - 0x41 + 10) * 10
				}
				if rxdata[13] >= '0' && rxdata[13] <= '9' {
					topRoomTemper = topRoomTemper + (rxdata[13] - 0x30)
				}
				if rxdata[13] >= 'a' && rxdata[13] <= 'f' {
					topRoomTemper = topRoomTemper + (rxdata[13] - 0x61 + 10)
				}
				if rxdata[13] >= 'A' && rxdata[13] <= 'F' {
					topRoomTemper = topRoomTemper + (rxdata[13] - 0x41 + 10)
				}

				//上門溫度,16進位...
				topRoomTemper = topRoomTemper + 2
				fmt.Printf("topRoomTemper: %d\n", topRoomTemper)
				awsTxData[4] = topRoomTemper & 0x0F
				if awsTxData[4] >= 0 && awsTxData[4] <= 9 {
					awsTxData[4] = awsTxData[4] + 0x30
				}
				if awsTxData[4] >= 10 && awsTxData[4] <= 15 {
					awsTxData[4] = awsTxData[4] - 10 + 0x61
				}
				awsTxData[3] = topRoomTemper >> 4
				if awsTxData[3] >= 0 && awsTxData[3] <= 9 {
					awsTxData[3] = awsTxData[3] + 0x30
				}
				if awsTxData[3] >= 10 && awsTxData[3] <= 15 {
					awsTxData[3] = awsTxData[3] - 10 + 0x61
				}
				//中門溫度...
				if rxdata[14] >= '0' && rxdata[14] <= '9' {
					minRoomTemper = (rxdata[14] - 0x30) * 10
				}
				if rxdata[14] >= 'a' && rxdata[14] <= 'f' {
					minRoomTemper = (rxdata[14] - 0x61 + 10) * 10
				}
				if rxdata[14] >= 'A' && rxdata[14] <= 'F' {
					minRoomTemper = (rxdata[14] - 0x41 + 10) * 10
				}
				if rxdata[15] >= '0' && rxdata[15] <= '9' {
					minRoomTemper = minRoomTemper + (rxdata[15] - 0x30)
				}
				if rxdata[15] >= 'a' && rxdata[15] <= 'f' {
					minRoomTemper = minRoomTemper + (rxdata[15] - 0x61 + 10)
				}
				if rxdata[15] >= 'A' && rxdata[15] <= 'F' {
					minRoomTemper = minRoomTemper + (rxdata[15] - 0x41 + 10)
				}
				//中門溫度,16進位...
				minRoomTemper = minRoomTemper - 2
				fmt.Printf("minRoomTemper: %d\n", minRoomTemper)
				awsTxData[6] = minRoomTemper & 0x0F
				if awsTxData[6] >= 0 && awsTxData[6] <= 9 {
					awsTxData[6] = awsTxData[6] + 0x30
				}
				if awsTxData[6] >= 10 && awsTxData[6] <= 15 {
					awsTxData[6] = awsTxData[6] - 10 + 0x61
				}
				awsTxData[5] = minRoomTemper >> 4
				if awsTxData[5] >= 0 && awsTxData[5] <= 9 {
					awsTxData[5] = awsTxData[5] + 0x30
				}
				if awsTxData[5] >= 10 && awsTxData[5] <= 15 {
					awsTxData[5] = awsTxData[5] - 10 + 0x61
				}

				//下門溫度...
				if rxdata[16] >= '0' && rxdata[16] <= '9' {
					bottomRoomTemper = (rxdata[16] - 0x30) * 10
				}
				if rxdata[16] >= 'a' && rxdata[16] <= 'f' {
					bottomRoomTemper = (rxdata[16] - 0x61 + 10) * 10
				}
				if rxdata[16] >= 'A' && rxdata[16] <= 'F' {
					bottomRoomTemper = (rxdata[16] - 0x41 + 10) * 10
				}
				if rxdata[17] >= '0' && rxdata[17] <= '9' {
					bottomRoomTemper = bottomRoomTemper + (rxdata[17] - 0x30)
				}
				if rxdata[17] >= 'a' && rxdata[17] <= 'f' {
					bottomRoomTemper = bottomRoomTemper + (rxdata[17] - 0x61 + 10)
				}
				if rxdata[17] >= 'A' && rxdata[17] <= 'F' {
					bottomRoomTemper = bottomRoomTemper + (rxdata[17] - 0x41 + 10)
				}
				//下門溫度,16進位...
				bottomRoomTemper = bottomRoomTemper - 4
				fmt.Printf("bottomRoomTemper: %d\n", bottomRoomTemper)
				awsTxData[8] = bottomRoomTemper & 0x0F
				if awsTxData[8] >= 0 && awsTxData[8] <= 9 {
					awsTxData[8] = awsTxData[8] + 0x30
				}
				if awsTxData[8] >= 10 && awsTxData[8] <= 15 {
					awsTxData[8] = awsTxData[8] - 10 + 0x61
				}
				awsTxData[7] = bottomRoomTemper >> 4
				if awsTxData[7] >= 0 && awsTxData[7] <= 9 {
					awsTxData[7] = awsTxData[7] + 0x30
				}
				if awsTxData[7] >= 10 && awsTxData[7] <= 15 {
					awsTxData[7] = awsTxData[7] - 10 + 0x61
				}

				//minRoomTemper
				//bottomRoomTemper
				//roomTemperAve

			}

			//Room temperature setting...
			//awsTxData[6] = rxdata[56]
			//awsTxData[7] = rxdata[57]
			//Room temperature...
			//awsTxData[8] = rxdata[58]
			//awsTxData[9] = rxdata[59]
			//power consumption
			//*256...
			if rxdata[4] >= '0' && rxdata[4] <= '9' {
				powerMetertemp = (rxdata[4] - '0')
			} else if rxdata[4] >= 'a' && rxdata[4] <= 'f' {
				powerMetertemp = (rxdata[4] - 'a') + 10
			}
			powerMeterH = uint16(powerMetertemp) << 4

			//	rxdata[5]
			if rxdata[5] >= '0' && rxdata[5] <= '9' {
				powerMetertemp = (rxdata[5] - '0')
			} else if rxdata[5] >= 'a' && rxdata[5] <= 'f' {
				powerMetertemp = (rxdata[5] - 'a') + 10
			}
			powerMeterH = powerMeterH + uint16(powerMetertemp)
			powerMeterH = powerMeterH * 256

			fmt.Printf("powerMeterH: %d\n", powerMeterH)

			//*1...
			if rxdata[6] >= '0' && rxdata[6] <= '9' {
				powerMetertemp = (rxdata[6] - '0')
			} else if rxdata[6] >= 'a' && rxdata[6] <= 'f' {
				powerMetertemp = (rxdata[6] - 'a') + 10
			}
			powerMeterL = uint16(powerMetertemp) << 4

			//*1...
			if rxdata[7] >= '0' && rxdata[7] <= '9' {
				powerMetertemp = (rxdata[7] - '0')
			} else if rxdata[7] >= 'a' && rxdata[7] <= 'f' {
				powerMetertemp = (rxdata[7] - 'a') + 10
			}
			powerMeterL = powerMeterL + uint16(powerMetertemp)

			fmt.Printf("powerMeterL: %d\n", powerMeterL)

			//大於 205,reset...
			powerMeter = (powerMeterH + powerMeterL)
			if powerMeter > 250 {
				powerIndex = powerMeter / 2500
				fmt.Printf("powerIndex %d\n", powerIndex)
				powerMeter = powerMeter - powerIndex*2500
			}
			//powerMeter = (powerMeterH + powerMeterL) / 10
			powerMeter = powerMeter / 10
			fmt.Printf("powerMeter/10: %d\n", powerMeter)

			//*65536..
			//	rxdata[6]
			//*16777216...
			//	rxdata[7]
			//powerMetertemp16 = powerMeter & 0x000f
			//awsTxData[10] = '0'

			powerMetertemp = uint8(powerMeter) & 0xf0
			powerMetertemp = powerMetertemp >> 4

			if powerMetertemp >= 0 && powerMetertemp <= 9 {
				awsTxData[9] = 0x30 + powerMetertemp
			} else if powerMetertemp >= 10 && powerMetertemp <= 15 {
				powerMetertemp = powerMetertemp - 10
				awsTxData[9] = 0x61 + powerMetertemp
			}

			//fmt.Printf("powerMetertemp_awsTxData[10]: %d\n", awsTxData[10])

			powerMetertemp = uint8(powerMeter) & 0x0f
			if powerMetertemp >= 0 && powerMetertemp <= 9 {
				awsTxData[10] = 0x30 + powerMetertemp
			} else if powerMetertemp >= 10 && powerMetertemp <= 15 {
				powerMetertemp = powerMetertemp - 10
				awsTxData[10] = 0x61 + powerMetertemp
			}

			//fmt.Printf("powerMetertemp_awsTxData[11]: %s\n", awsTxData[11])

			//error code...
			awsTxData[11] = '0'
			awsTxData[12] = '0'
			awsTxData[13] = '0'
			awsTxData[14] = '0'
			awsTxData[15] = '0'
			awsTxData[16] = '0'
			awsTxData[17] = '0'
			awsTxData[18] = '0'
			awsTxData[19] = '0'
			awsTxData[20] = '0'
			awsTxData[21] = '0'
			fmt.Printf("water_temp_in: %s\n", string(water_temp_in[0:4]))
			awsstr = strings.Replace(awsstr, "######################", string(awsTxData[:]), -1)

		} //商冰0x02結束...

		if rxdata[0] == '0' && rxdata[1] == '3' {
			awsTxData[0] = '3'
			awsTxData[1] = '0'
			//HCHO
			awsTxData[2] = rxdata[4]
			awsTxData[3] = rxdata[5]
			awsTxData[4] = rxdata[6]
			awsTxData[5] = rxdata[7]
			//Co2
			awsTxData[6] = rxdata[8]
			awsTxData[7] = rxdata[9]
			awsTxData[8] = rxdata[10]
			awsTxData[9] = rxdata[11]
			//Co
			awsTxData[10] = rxdata[20]
			awsTxData[11] = rxdata[21]
			awsTxData[12] = rxdata[22]
			awsTxData[13] = rxdata[23]

			//遠傳測試,針對7560過濾....
			if rxMac[14] == '6' && rxMac[15] == '0' {

				//Co,上傳次數...
				awsTxData[10] = rxdata[20]
				awsTxData[11] = rxdata[21]
				awsTxData[12] = rxdata[58]
				awsTxData[13] = rxdata[59]
			}

			//PM2.5
			awsTxData[14] = rxdata[28]
			awsTxData[15] = rxdata[29]
			awsTxData[16] = rxdata[30]
			awsTxData[17] = rxdata[31]
			//遠傳測試,針對7560過濾....
			if rxMac[14] == '6' && rxMac[15] == '0' {

				//PM2.5,RSSI...
				awsTxData[14] = rxdata[28]
				awsTxData[15] = rxdata[29]
				awsTxData[16] = rxdata[56]
				awsTxData[17] = rxdata[57]
			}

			//Temperature
			awsTxData[18] = rxdata[12]
			awsTxData[19] = rxdata[13]
			awsTxData[20] = rxdata[14]
			awsTxData[21] = rxdata[15]
			awsstr = strings.Replace(awsstr, "######################", string(awsTxData[:]), -1)
		} //空氣偵測器結束...

		//建立mqtt結束...
		//var arr [10]rune
		StringToRuneArr(awsstr, txdata[:])

		fmt.Println(string(txdata[:]))

		//Publish 5 messages to /go-mqtt/sample at qos 1 and wait for the receipt
		//from the server after sending each message
		//fmt.Printf("txdata: %s\n", txdata)
		//text := fmt.Sprintf("%s", txdata)
		awsPublishToken := awsClient.Publish(awsTopic, 0, false, string(txdata[:]))
		//awsToken.Wait()
		for !awsPublishToken.WaitTimeout(3 * time.Second) {

		}
		if err := awsPublishToken.Error(); err != nil {
			log.Fatal(err)
			fmt.Printf("awsPublishToken error\n")
		} else {
			fmt.Printf("Aws push finish\n")
		}

		//建立mqtt結束...
		awsClient.Unsubscribe(awsTopic)
		fmt.Printf("awsCliend is closed\n")
		awsClientEndToken := awsClient.Unsubscribe(awsTopic)
		for !awsClientEndToken.WaitTimeout(3 * time.Second) {

		}
		if err := awsClientEndToken.Error(); err != nil {
			log.Fatal(err)
			fmt.Printf("awsClientEndToken error\n")
		}
		awsClient.Disconnect(250)
		/*
			if awsToken := awsClient.Unsubscribe(awsTopic); awsToken.Wait() && awsToken.Error() != nil {
				fmt.Println(awsToken.Error())
			}
		*/
		//time.Sleep(5 * time.Second)

	}

	/*
		//	fmt.Printf("check: %t\n", (s == "data"))
		if s == "on" {
			fmt.Println("on is received!")
			//	TurnAllOn()
		} else if s == "off" {
			fmt.Println("off is received!")
			//	GlowOff()
		}
	*/
	//	fmt.Println(strings.Contains(msg.Payload(), "data")) //true
}

var awsfun MQTT.MessageHandler = func(client MQTT.Client, msg MQTT.Message) {
	//	mQTTMessage := &MQTTMessage{msg, m}
	//	fmt.Printf("TOPIC: %s\n", msg.Topic())
	//var indexAwsMessage int
	awsKnt++
	if awsKnt > 65530 {
		awsKnt = 0
	}
	fmt.Printf("awsMSG: %d\n", awsKnt)
}

/*
var msgRcvd := func(client *mqtt.Client, message mqtt.Message) {
	fmt.Printf("Received message on topic: %s\nMessage: %s\n", message.Topic(), message.Payload())
}
*/

// getRandomClientId returns randomized ClientId.
func getRandomClientId() string {
	const alphanum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	var bytes = make([]byte, MaxClientIdLen)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = alphanum[b%byte(len(alphanum))]
	}
	return "ClientID-" + string(bytes)
}

//influx tets
/*
func connInflux() client.Client {
	cli, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     "http://localhost:8086",
		Username: username,
		Password: password,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("influx pass\n")
	return cli

}

*/
func main() {
	Knt = 0
	/*
		//influxdb測試...
		// Create a new HTTPClient
		c, err := client.NewHTTPClient(client.HTTPConfig{
			Addr:     "http://localhost:8086",
			Username: username,
			Password: password,
		})
		if err != nil {
			log.Fatal(err)
		}
		defer c.Close()

		fmt.Printf("pass http\n")

		// Create a new point batch
		bp, err := client.NewBatchPoints(client.BatchPointsConfig{
			Database:  MyDB,
			Precision: "s",
		})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("pass creat db\n")

		data[0] = 27.8
		data[1] = 26.8
		data[2] = 14.6
		data[3] = 16.7
		data[4] = 33.8
		data[5] = 54.6
		data[6] = 4.15
		data[7] = 4.18
		data[8] = 7.75
		data[9] = 2.8
		data[10] = 380.0
		data[11] = 0.0
		data[12] = 80.8

		// Create a point and add to batch
		tags := map[string]string{"location": "bank"}
		fields := map[string]interface{}{
			"water_temp_in":      data[0],
			"water_temp_out":     data[1],
			"ice_temp_in":        data[2],
			"ice_temp_out":       data[3],
			"left_temp_out":      data[4],
			"right_temp_out":     data[5],
			"left_pressor_high":  data[6],
			"left_pressor_low":   data[7],
			"right_pressor_high": data[8],
			"right_pressor_low":  data[9],
			"voltage":            data[10],
			"left_amp":           data[11],
			"right_amp":          data[12],
		}

		pt, err := client.NewPoint("MAC_013157800087578", tags, fields, time.Now())
		if err != nil {
			log.Fatal(err)
		}
		bp.AddPoint(pt)

		// Write the batch
		if err := c.Write(bp); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("pass write\n")
		// Close client resources
		if err := c.Close(); err != nil {
			log.Fatal(err)
		}
	*/

	//建立getRandomClientId returns randomized ClientId.
	gcpclientId := getRandomClientId()
	fmt.Printf("gcpclientId: %s\n", gcpclientId)
	//建立Go的監聴封包...
	sigc := make(chan os.Signal, 1)

	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)

	//create a ClientOptions struct setting the broker address, clientid, turn
	//off trace output and set the default message handler
	//opts := MQTT.NewClientOptions().AddBroker("tcp://140.124.182.66:1883")
	gcpMQTTBroker := MQTT.NewClientOptions().AddBroker("tcp://35.201.226.72:1883")
	//opts := MQTT.NewClientOptions().AddBroker("tcp://13.114.3.126:1883")
	//opts.SetClientID("u89-0001")
	gcpMQTTBroker.SetClientID(gcpclientId)
	gcpMQTTBroker.SetDefaultPublishHandler(f)
	gcpMQTTBroker.SetAutoReconnect(true)
	gcpMQTTBroker.SetCleanSession(false)
	//

	//topic := "GIOT-GW/UL/1C497BE1FD99"
	//topic := "iaq"
	gcpTopic := "NB-IoT"
	//create and start a client using the above ClientOptions
	gcpClient := MQTT.NewClient(gcpMQTTBroker)
	if token := gcpClient.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	} else {
		fmt.Printf("Connected to 35.201.226.72 KU server\n")
	}

	//subscribe to the topic "iaq" and request messages to be delivered
	//at a maximum qos of zero, wait for the receipt to confirm the subscription
	if token := gcpClient.Subscribe(gcpTopic, 0, nil); token.Wait() && token.Error() != nil {
		fmt.Println(token.Error())
		//	fmt.Printf("Received message on topic: %s\nMessage: %s\n", message.Topic(), message.Payload())
		token.Wait()
		//	os.Exit(1)
	}

	//Publish 5 messages to /go-mqtt/sample at qos 1 and wait for the receipt
	//from the server after sending each message
	/*
		for i := 0; i < 1; i++ {
			text := fmt.Sprintf("this is msg #%d!\n", i)
			token := client.Publish(topic, 0, false, text)
			token.Wait()
		}

		time.Sleep(5 * time.Second)
	*/

	//當到達 時間,結束MQTT section...

	//unsubscribe from /go-mqtt/sample
	/*
		if token := c.Unsubscribe("GIOT-GW/UL/1C497BE1FD99"); token.Wait() && token.Error() != nil {
			fmt.Println(token.Error())
			os.Exit(1)
		}
	*/
	/*
		//建立第二個MQTT Cliend...
		awsTopic := "iaq-test"
		awsMQTTBroker := MQTT.NewClientOptions().AddBroker("tcp://13.114.3.126:1883")
		awsMQTTBroker.SetClientID(clientId)
		awsMQTTBroker.SetDefaultPublishHandler(awsfun)

		//create and start a client using the above ClientOptions
		awsClient := MQTT.NewClient(awsMQTTBroker)
		if awsToken := awsClient.Connect(); awsToken.Wait() && awsToken.Error() != nil {
			panic(awsToken.Error())
		} else {
			fmt.Printf("Connected to 13.114.3.126 server\n")
		}

		if awsToken := awsClient.Subscribe(awsTopic, 0, nil); awsToken.Wait() && awsToken.Error() != nil {
			fmt.Println(awsToken.Error())
			awsToken.Wait()
			//	os.Exit(1)
		}
		//Publish 5 messages to /go-mqtt/sample at qos 1 and wait for the receipt
		//from the server after sending each message
		for i := 0; i < 1; i++ {
			text := fmt.Sprintf("this is msg #%d!\n", i)
			awsToken := awsClient.Publish(awsTopic, 0, false, text)
			awsToken.Wait()
		}

		time.Sleep(5 * time.Second)
	*/
	// Wait for receiving a signal.
	//<-sigc
	s := <-sigc
	fmt.Println("Got signal:", s) //Got signal: terminated
	//	c.Disconnect(11250)
}
