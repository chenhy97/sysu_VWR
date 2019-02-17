import json
import random
import warnings
import pandas as pd
import matplotlib.pyplot as plt
import statsmodels.api as sm
from statsmodels.tsa.arima_model import ARMA
from statsmodels.tsa.stattools import adfuller

colors = [ "#DC143C", "#800080", "#0000FF", "#708090", "#00FFFF", "#7FFFAA", "#8B0000", "#FF8C00", "#FFFF00", "#006400"]
def randomColor(choice):
    index = int(random.random() * len(choice))
    color = choice[index]
    del(choice[index])
    return color

def pre_process_data(raw_data, noise_control=10000, rolling=True):
    format_data = []
    single_trace = {}
    before_data = {}
    pre_id = ''
    delay_timestamp = 0
    for trace_data in raw_data:
        if trace_data['name'] == 'Delay':
            print('delay:', trace_data['annotations'][0]['timestamp'])
        # format raw data with traceId
        if pre_id == trace_data['traceId'] and len(before_data['annotations']) >= 2:
            # the second condition filters the data influenced by chaosmonkey
            single_trace[before_data['id']] = before_data
            # print(pre_id, single_trace)
            # if 'parentId' not in before_data:
            #     # record root request
            #     single_trace['root'] = before_data['id']
        else:
            # print(len(single_trace), single_trace)
            if len(single_trace) > 1:
                # filter the trace with only one data (but has another data recorded the root request)
                single_trace[before_data['id']] = before_data
                # record the last request to build the request tree (because the data contains parentid)
                single_trace['last'] = before_data['id']
                format_data.append(single_trace)
                # print(len(single_trace), single_trace)
                single_trace = {}

        before_data = trace_data
        pre_id = trace_data['traceId']
    if len(single_trace) > 1:
        # proccess the last data
        single_trace[before_data['id']] = before_data
        format_data.append(single_trace)

    # control node data printing
    if_print = False
    # control delay data when printing(delay longer than setting value will be printed)
    print_control = 500

    anomaly = {}
    delay_data = {}
    nodes = {}
    relation = {}
    # process every trace
    for trace in format_data:
        # process single trace
        for single_trace_key in trace:
            # the last key records the trace's last traceid, should be filtered
            if single_trace_key != 'last':
                single_trace = trace[single_trace_key]
                annotations = single_trace['annotations']
                if len(annotations) == 4:
                    node_data = {}
                    # the single trace has the data of CS, SR, SS, CR, process the annontation
                    for annotation in annotations:
                        endpoint = annotation['endpoint']
                        type = annotation['value']
                        ip = str(endpoint['ipv4'])
                        # source = ''
                        # check if the ip in nodes dictionary
                        if ip in nodes:
                            node_data = nodes[ip]
                        else:
                            node_data = endpoint
                        # check if the ip in delay_data dictionary
                        if ip not in delay_data:
                            delay_data[ip] = {'server': [], 'server_t': [], 'client': [], 'client_t': []}
                        if type == 'cs':
                            node_data['cs'] = annotation['timestamp']
                            source = ip
                        elif type == 'sr':
                            node_data['sr'] = annotation['timestamp']
                            # store the source -> targe relationship
                            if source in relation:
                                if ip not in relation[source]:
                                    relation[source].append(ip)
                            else:
                                relation[source] = [ip]
                        elif type == 'ss' and 'sr' in node_data:
                            # append delay data to the correspond ip's serial
                            delay = annotation['timestamp'] - node_data['sr']
                            if delay <= noise_control:
                                delay_data[ip]['server'].append(delay)
                                delay_data[ip]['server_t'].append(annotation['timestamp'])
                            else:
                                # apply threshold value to filter the anomaly data
                                temp = {
                                    'ip': ip,
                                    'type': 'server',
                                    'timestamp': annotation['timestamp'],
                                    'delay': delay
                                }
                                if single_trace['traceId'] in anomaly:
                                    anomaly[single_trace['traceId']].append(temp)
                                else:
                                    anomaly[single_trace['traceId']] = [temp]                               # print(ip, 'server', annotation['timestamp'], single_trace['traceId'], delay)
                            if delay > print_control and if_print:
                                print('traceid:', single_trace['id'], ip, ', server:',
                                      annotation['timestamp'] - node_data['sr'], 'avg:', node_data['duration_server'])
                        elif type == 'cr' and 'cs' in node_data:
                            # append delay data to the correspond ip's serial
                            delay = annotation['timestamp'] - node_data['cs']
                            if delay <= noise_control:
                                delay_data[ip]['client'].append(delay)
                                delay_data[ip]['client_t'].append(annotation['timestamp'])
                            else:
                                # apply threshold value to filter the anomaly data
                                temp = {
                                    'ip': ip,
                                    'type': 'client',
                                    'timestamp': annotation['timestamp'],
                                    'delay': delay
                                }
                                if single_trace['traceId'] in anomaly:
                                    anomaly[single_trace['traceId']].append(temp)
                                else:
                                    anomaly[single_trace['traceId']] = [temp]
                                # print(ip, 'client', annotation['timestamp'], single_trace['traceId'], delay)
                            if delay > print_control and if_print:
                                print('traceid:', single_trace['id'], ip, ', client:',
                                      annotation['timestamp'] - node_data['cs'], 'avg:', node_data['duration_client'])
                        nodes[ip] = node_data
    print('Anomaly:\n', anomaly)
    print('Relation:\n', relation)
    # calculate mean and std for every ip & every type
    for ip in delay_data:
        # server_serial = delay_data[ip]['server']
        if len(delay_data[ip]['server']) == 0:
            nodes[ip]['duration_server'] = 0
            nodes[ip]['std_server'] = 0
        else:
            # put the series data in order (to pandas series)
            # print(delay_data[ip]['server_t'])
            series = pd.Series(delay_data[ip]['server'], index=pd.to_datetime(delay_data[ip]['server_t'], unit='us'))
            # print(ip, min(delay_data[ip]['server_t']), max(delay_data[ip]['server_t']))
            # roll with the window of 1s
            if rolling:
                series = series.sort_index().rolling('1S').mean()
            delay_data[ip]['server'] = series
            # calulate server serial statistical characteristic
            nodes[ip]['duration_server'] = series.mean()
            nodes[ip]['std_server'] = series.std()
            # test data stationarity
            # print('\nChecking ip:', ip, 'server serial data')
            # test_stationarity(delay_data[ip]['server'])
        # client_serial = delay_data[ip]['client']
        if len(delay_data[ip]['client']) == 0:
            nodes[ip]['duration_client'] = 0
            nodes[ip]['std_client'] = 0
        else:
            # put the series data in order (to pandas series)
            series = pd.Series(delay_data[ip]['client'], index=pd.to_datetime(delay_data[ip]['client_t'], unit='us'))
            # roll with the window of 1s
            if rolling:
                series = series.sort_index().rolling('1S').mean()
            delay_data[ip]['client'] = series
            # calulate client serial statistical characteristic
            nodes[ip]['duration_client'] = series.mean()
            nodes[ip]['std_client'] = series.std()
            # test data stationarity
            # print('\nChecking ip:', ip, 'client serial data')
            # test_stationarity(delay_data[ip]['client'])
    return nodes, delay_data

# draw probability density plot for each ip
def draw_probability_chart():
    choice1 = colors[:]
    for ip in delay_data:
        plt.title('probability density plot for ' + ip)
        if len(delay_data[ip]['server']) > 0:
            plt.hist(delay_data[ip]['server'], 12, normed=True, color=randomColor(choice1), alpha=.5,
                     label=ip + '_server')
        if len(delay_data[ip]['client']) > 0:
            plt.hist(delay_data[ip]['client'], 12, normed=True, color=randomColor(choice1), alpha=.5,
                     label=ip + '_client')
        plt.legend(loc=0)
        plt.figure()
    plt.show()

# draw delay data line chart for each type
def draw_delay_line_chart():
    plt.figure(1)
    plt.title('node delay chart as server')
    plt.xlabel('data size')
    plt.ylabel('delay(ms)')
    plt.grid()
    plt.figure(2)
    plt.title('node delay chart as client')
    plt.xlabel('data size')
    plt.ylabel('delay(ms)')
    plt.grid()
    plt.figure(3)
    plt.title('overall node delay chart')
    plt.xlabel('data size')
    plt.ylabel('delay(ms)')
    plt.grid()
    for ip in delay_data:
        if len(delay_data[ip]['server']) > 0:
            # x = list(range(1, len(delay_data[ip]['server']) + 1))
            plt.figure(1)
            plt.plot(delay_data[ip]['server'], label=ip)
            # plt.plot(x, [nodes[ip]['duration_server']] * len(x), color)
            plt.figure(3)
            plt.plot(delay_data[ip]['server'], label=ip + '_server')
        if len(delay_data[ip]['client']) > 0:
            # x = list(range(1, len(delay_data[ip]['client']) + 1))
            plt.figure(2)
            plt.plot(delay_data[ip]['client'], label=ip)
            # plt.plot(x, [nodes[ip]['duration_client']] * len(x), color)
            plt.figure(3)
            plt.plot(delay_data[ip]['client'], label=ip + '_client')
    plt.figure(1)
    plt.legend(bbox_to_anchor=[0.3, 1])
    plt.figure(2)
    plt.legend(bbox_to_anchor=[0.3, 1])
    plt.figure(3)
    plt.legend(loc='upper center', fontsize=7)
    plt.show()

#Dickey-Fuller test:
def test_stationarity(timeseries):
    print('Results of Dickey-Fuller Test:')
    dftest = adfuller(timeseries, autolag='AIC')
    # dftest的输出前一项依次为检测值，p值，滞后数，使用的观测数，各个置信度下的临界值
    dfoutput = pd.Series(dftest[0:4], index=['Test Statistic', 'p-value', '#Lags Used', 'Number of Observations Used'])
    for key, value in dftest[4].items():
        dfoutput['Critical value (%s)' % key] = value

    print(dfoutput)

def draw_acf_pacf(data):
    fig = plt.figure(figsize=(12, 8))
    ax1 = fig.add_subplot(211)
    fig = sm.graphics.tsa.plot_acf(data.dropna(), lags=40, ax=ax1)
    ax2 = fig.add_subplot(212)
    fig = sm.graphics.tsa.plot_pacf(data.dropna(), lags=40, ax=ax2)
    plt.show()

warnings.filterwarnings('ignore')

file = open(r'/Users/chen/go/src/github.com/adrianco/spigo/json_metrics/sockshop_flow.json', 'r').read()
raw_data = json.loads(file)

nodes, delay_data = pre_process_data(raw_data, rolling=True)
for ip in nodes:
    print(nodes[ip])

# MAX_AR = 3
# MAX_MA = 3
# DIFF = 0
# for ip in nodes:
#     print(ip, ':', nodes[ip])
#     if nodes[ip]['duration_client'] > 0:
#         data = delay_data[ip]['client']
#     else:
#         data = delay_data[ip]['server']
#     if DIFF > 0:
#         order = sm.tsa.arma_order_select_ic(data.diff(DIFF).dropna(), max_ar=MAX_AR, max_ma=MAX_MA, ic=['aic', 'bic'])
#     else:
#         order = sm.tsa.arma_order_select_ic(data, max_ar=MAX_AR, max_ma=MAX_MA, ic=['aic', 'bic'])
#     print(ip, ': aic_min_order', order.aic_min_order, ' bic_min_order', order.bic_min_order)
#     arma_mod = ARMA(data.astype('float64'), (order.aic_min_order[0], DIFF, order.aic_min_order[1])).fit(disp=-1, method='mle')
#
#     summary = (arma_mod.summary(alpha=.05))
#     # get std err
#     std_err = float(summary.as_csv().split('\n')[9].split(',')[2])
#     print(std_err)
#     plt.figure(ip)
#     plt.plot(data, color='blue', label='raw_data')
#     plt.scatter(arma_mod.fittedvalues.index, arma_mod.fittedvalues, color='red', label='arma_data')
#     plt.title('%s std_err: %f diff:%d' % (ip, std_err, DIFF))
#     plt.legend(loc=0)
#     plt.show()

# 画时间序列图
draw_delay_line_chart()
# test_stationarity(delay_data['54.198.0.5']['client'])


